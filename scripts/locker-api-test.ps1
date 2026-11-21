# 智能快寄柜接口自动化验收脚本
# 覆盖 22 个接口，每个接口至少连续 3 轮；证据落 docs/locker/test-evidence/
# 依赖：本机 MySQL(127.0.0.1:3306, root/root, 库 bw_locker) + Redis(127.0.0.1:6379) + gateway(127.0.0.1:8080)
param(
    [string]$BaseUrl = "http://127.0.0.1:8080",
    [string]$User = "u_1001",
    [int]$Rounds = 3
)

$ErrorActionPreference = "Stop"
$ProgressPreference = "SilentlyContinue"

$root = Split-Path -Parent $PSScriptRoot
$evidenceDir = Join-Path $root "docs\locker\test-evidence"
New-Item -ItemType Directory -Force -Path $evidenceDir | Out-Null
$mysql = "C:\Program Files\MySQL\MySQL Server 8.0\bin\mysql.exe"
$stamp = Get-Date -Format "yyyyMMdd-HHmmss"

$records = [System.Collections.Generic.List[object]]::new()

function Invoke-Api {
    param(
        [string]$Name,
        [string]$Method,
        [string]$Path,
        [object]$Body,
        [int]$Expect = 200
    )
    $url = "$BaseUrl/api/v1/locker$Path"
    $sw = [System.Diagnostics.Stopwatch]::StartNew()
    $status = 0
    $content = ""
    try {
        $params = @{
            Uri         = $url
            Method      = $Method
            Headers     = @{ "X-User-Id" = $User }
            TimeoutSec  = 15
            UseBasicParsing = $true
        }
        if ($null -ne $Body) {
            $params.ContentType = "application/json"
            $params.Body = ($Body | ConvertTo-Json -Depth 8 -Compress)
        }
        $resp = Invoke-WebRequest @params
        $status = [int]$resp.StatusCode
        $content = $resp.Content
    } catch {
        if ($null -ne $_.Exception.Response) {
            $status = [int]$_.Exception.Response.StatusCode
            $content = $_.ErrorDetails.Message
            if ([string]::IsNullOrWhiteSpace($content)) {
                $content = $_.Exception.Message
            }
        } else {
            $status = 0
            $content = $_.Exception.Message
        }
    }
    $sw.Stop()

    $ok = ($status -eq $Expect)
    $records.Add([pscustomobject]@{
            name       = $Name
            method     = $Method
            path       = $Path
            expected   = $Expect
            status     = $status
            ok         = $ok
            latency_ms = [int]$sw.ElapsedMilliseconds
            response   = $content
        })
    $tag = if ($ok) { "PASS" } else { "FAIL" }
    Write-Host ("[{0}] {1,-46} {2,-4} {3,3} (expect {4,3}) {5,5}ms" -f $tag, $Name, $Method, $status, $Expect, $sw.ElapsedMilliseconds)
    return $content
}

function Data-Of($content) {
    if ([string]::IsNullOrWhiteSpace($content)) { return $null }
    try { return ($content | ConvertFrom-Json).data } catch { return $null }
}

function Endpoint-Key([string]$method, [string]$path) {
    $p = ($path -split '\?')[0]
    $p = $p -replace '/orders/(?:LK|PAY)[0-9A-Z]+', '/orders/:order_no'
    $p = $p -replace '/grants/GR[0-9A-Z]+', '/grants/:grant_no'
    $p = $p -replace '/notices/NT[0-9A-Z]+', '/notices/:id'
    return "$method $p"
}

function Force-Overdue([string]$orderNo) {
    & $mysql --host=127.0.0.1 --port=3306 --user=root --password=root --database=bw_locker --silent `
        --execute="UPDATE lk_orders SET start_at=DATE_SUB(NOW(), INTERVAL 3 HOUR), end_at=DATE_SUB(NOW(), INTERVAL 2 HOUR) WHERE order_no='$orderNo';" | Out-Null; if ($LASTEXITCODE -ne 0) { throw "force overdue failed for $orderNo" }
}

$roundPlan = @(
    @{ A = "cab_001_02"; B = "cab_001_05"; SA = "st_001"; SB = "st_001"; Coupon = "cp_001"; Recharge = 10000 },
    @{ A = "cab_001_08"; B = "cab_002_02"; SA = "st_001"; SB = "st_002"; Coupon = "cp_002"; Recharge = 5000 },
    @{ A = "cab_002_05"; B = "cab_003_02"; SA = "st_002"; SB = "st_003"; Coupon = $null;  Recharge = 2000 }
)

for ($r = 1; $r -le $Rounds; $r++) {
    $plan = $roundPlan[($r - 1) % $roundPlan.Count]
    Write-Host ""
    Write-Host "===== Round $r =====" -ForegroundColor Cyan

    # --- 首页与网点（4） ---
    Invoke-Api "round$r-cities" "GET" "/cities" $null 200 | Out-Null
    Invoke-Api "round$r-stations" "GET" "/stations?city_id=city_512&lat=33.9631&lng=118.2752" $null 200 | Out-Null
    Invoke-Api "round$r-cabinets" "GET" "/stations/st_001/cabinets" $null 200 | Out-Null
    Invoke-Api "round$r-guide" "GET" "/guide" $null 200 | Out-Null

    # --- 试算（1） ---
    $quoteBody = @{ size = "M"; hours = 5 }
    if ($plan.Coupon) { $quoteBody.coupon_id = $plan.Coupon }
    Invoke-Api "round$r-quote" "POST" "/quote" $quoteBody 200 | Out-Null

    # --- 下单 A ---
    $orderBody = @{ station_id = $plan.SA; cabinet_id = $plan.A; hours = 5 }
    if ($plan.Coupon) { $orderBody.coupon_id = $plan.Coupon }
    $created = Invoke-Api "round$r-create-order-A" "POST" "/orders" $orderBody 201
    $orderA = (Data-Of $created).order_no
    $payA = (Data-Of $created).pay

    if ($r -eq 1) {
        # 并发/重复下单：柜门已锁定，第二次下单必须 409
        Invoke-Api "round$r-negative-cabinet-locked" "POST" "/orders" @{ station_id = $plan.SA; cabinet_id = $plan.A; hours = 5 } 409 | Out-Null
        # 金额不符回调必须 409
        Invoke-Api "round$r-negative-amount-mismatch" "POST" "/payments/notify" @{ pay_no = $payA.pay_no; order_no = $orderA; amount_fen = 1; success = $true } 409 | Out-Null
    }

    # --- 支付回调 A（成功，2 次调用验证幂等） ---
    $paidA = Invoke-Api "round$r-pay-A" "POST" "/payments/notify" @{ pay_no = $payA.pay_no; order_no = $orderA; amount_fen = $payA.amount_fen; success = $true } 200
    Invoke-Api "round$r-pay-A-duplicate" "POST" "/payments/notify" @{ pay_no = $payA.pay_no; order_no = $orderA; amount_fen = $payA.amount_fen; success = $true } 200 | Out-Null

    # --- 订单列表 / 详情（2） ---
    Invoke-Api "round$r-orders" "GET" "/orders?status=all" $null 200 | Out-Null
    Invoke-Api "round$r-order-detail-A" "GET" "/orders/$orderA" $null 200 | Out-Null

    # --- 临时授权（3 次核销，第 3 次应耗尽） ---
    $grant = Invoke-Api "round$r-grant-create" "POST" "/orders/$orderA/grants" $null 201
    $grantNo = (Data-Of $grant).grant_no
    Invoke-Api "round$r-grant-use-1" "POST" "/grants/$grantNo/use" $null 200 | Out-Null
    Invoke-Api "round$r-grant-use-2" "POST" "/grants/$grantNo/use" $null 200 | Out-Null
    Invoke-Api "round$r-grant-use-3-exhausted" "POST" "/grants/$grantNo/use" $null 409 | Out-Null

    # --- 正常取出 ---
    Invoke-Api "round$r-pickup-A" "POST" "/orders/$orderA/pickup" $null 200 | Out-Null

    # --- 订单 B：超时链路 ---
    $createdB = Invoke-Api "round$r-create-order-B" "POST" "/orders" @{ station_id = $plan.SB; cabinet_id = $plan.B; hours = 1 } 201
    $orderB = (Data-Of $createdB).order_no
    $payB = (Data-Of $createdB).pay
    Invoke-Api "round$r-pay-B" "POST" "/payments/notify" @{ pay_no = $payB.pay_no; order_no = $orderB; amount_fen = $payB.amount_fen; success = $true } 200 | Out-Null
    Force-Overdue $orderB
    Invoke-Api "round$r-orders-after-expire" "GET" "/orders?status=all" $null 200 | Out-Null
    Invoke-Api "round$r-negative-pickup-overdue" "POST" "/orders/$orderB/pickup" $null 409 | Out-Null
    Invoke-Api "round$r-overdue-pay" "POST" "/orders/$orderB/overdue-pay" $null 200 | Out-Null

    # --- 余额与充值（5） ---
    Invoke-Api "round$r-balance" "GET" "/balance" $null 200 | Out-Null
    Invoke-Api "round$r-flows" "GET" "/balance/flows" $null 200 | Out-Null
    $recharge = Invoke-Api "round$r-recharge-create" "POST" "/recharges" @{ amount_fen = $plan.Recharge; channel = "alipay" } 201
    $rcNo = (Data-Of $recharge).recharge_no
    Invoke-Api "round$r-recharge-notify" "POST" "/recharges/notify" @{ recharge_no = $rcNo; amount_fen = $plan.Recharge; success = $true } 200 | Out-Null
    if ($r -eq 1) {
        Invoke-Api "round$r-recharge-notify-duplicate" "POST" "/recharges/notify" @{ recharge_no = $rcNo; amount_fen = $plan.Recharge; success = $true } 200 | Out-Null
    }
    Invoke-Api "round$r-recharges" "GET" "/recharges" $null 200 | Out-Null

    # --- 个人中心与卡券（2） ---
    Invoke-Api "round$r-profile" "GET" "/profile" $null 200 | Out-Null
    Invoke-Api "round$r-coupons" "GET" "/coupons?status=all" $null 200 | Out-Null

    # --- 消息通知（2） ---
    $notices = Invoke-Api "round$r-notices" "GET" "/notices?unread_only=true" $null 200
    $unread = (Data-Of $notices).items
    $noticeId = $null
    if ($unread -and $unread.Count -gt 0) { $noticeId = $unread[0].id }
    if ($noticeId) {
        Invoke-Api "round$r-notice-read" "POST" "/notices/$noticeId/read" $null 200 | Out-Null
    } else {
        Invoke-Api "round$r-notices-fallback" "GET" "/notices" $null 200 | Out-Null
    }

    $roundRecords = $records | Where-Object { $_.name -like "round$r-*" }
    $file = Join-Path $evidenceDir ("round{0}-{1}.json" -f $r, $stamp)
    $roundRecords | ConvertTo-Json -Depth 6 | Set-Content -Encoding utf8 $file
    Write-Host ("round {0} evidence -> {1}" -f $r, $file) -ForegroundColor DarkGray
}

# --- 汇总 ---
$total = $records.Count
$failed = @($records | Where-Object { -not $_.ok })
$endpoints = @{}
foreach ($rec in $records) {
    $key = Endpoint-Key $rec.method $rec.path
    if (-not $endpoints.ContainsKey($key)) { $endpoints[$key] = @{ calls = 0; pass = 0 } }
    $endpoints[$key].calls++
    if ($rec.ok) { $endpoints[$key].pass++ }
}

$summary = [pscustomobject]@{
    generated_at = (Get-Date -Format "yyyy-MM-ddTHH:mm:sszzz")
    base_url     = $BaseUrl
    rounds       = $Rounds
    total_calls  = $total
    passed       = ($total - $failed.Count)
    failed       = $failed.Count
    endpoints    = ($endpoints.GetEnumerator() | ForEach-Object { [pscustomobject]@{ endpoint = $_.Key; calls = $_.Value.calls; pass = $_.Value.pass } } | Sort-Object endpoint)
}
$summaryFile = Join-Path $evidenceDir ("summary-$stamp.json")
$summary | ConvertTo-Json -Depth 6 | Set-Content -Encoding utf8 $summaryFile

Write-Host ""
Write-Host "===== SUMMARY =====" -ForegroundColor Cyan
$summary.endpoints | Format-Table -AutoSize | Out-String | Write-Host
Write-Host ("total={0} passed={1} failed={2}" -f $total, ($total - $failed.Count), $failed.Count)
if ($failed.Count -gt 0) {
    $failed | Format-Table name, status, expected | Out-String | Write-Host
    exit 1
}
Write-Host ("summary -> " + $summaryFile) -ForegroundColor Green
exit 0



