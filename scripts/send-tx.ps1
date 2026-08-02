param(
    [Parameter(Mandatory=$true)]
    [string]$TxFile,
    [string]$KeyFile = "wallet.json",
    [string]$RpcUrl = "http://localhost:8545"
)

# 0. Auto-resolve nonce from the node
Write-Host "=== Resolviendo nonce ===" -ForegroundColor Cyan
$txContent = Get-Content $TxFile -Raw | ConvertFrom-Json
$sender = $txContent.sender
Write-Host "   Sender: $sender" -ForegroundColor Gray

$nonceReq = @"
{"jsonrpc":"2.0","method":"dsn_getAccount","params":{"address":"$sender"},"id":1}
"@
$nonceFile = Join-Path $env:TEMP "dsn-nonce-req.json"
[IO.File]::WriteAllText($nonceFile, $nonceReq)
$nonceResp = curl.exe -s -X POST $RpcUrl -H "Content-Type: application/json" -d "@$nonceFile" 2>&1
Remove-Item $nonceFile -Force -ErrorAction SilentlyContinue
$nonceResult = $nonceResp | ConvertFrom-Json

if ($nonceResult.error) {
    Write-Host "ERROR consultando nonce: $($nonceResult.error.message)" -ForegroundColor Red
    exit 1
}

$currentNonce = $nonceResult.result.nonce
if ($currentNonce -eq $null) { $currentNonce = 0 }
# Protocol expects tx nonce = account nonce + 1
$txNonce = $currentNonce + 1
Write-Host "   Nonce on-chain: $currentNonce -> tx nonce: $txNonce" -ForegroundColor Yellow

# Update tx.json with correct nonce (timestamp is auto-set by wallet sign)
$txContent.nonce = $txNonce
$txContent | ConvertTo-Json | Set-Content $TxFile -Force
Write-Host "   Nonce: $txNonce -> $TxFile" -ForegroundColor Green

# 1. Sign the transaction
Write-Host "`n=== Firmando transaccion ===" -ForegroundColor Cyan
$signedOutput = & ".\dsn.exe" wallet sign $TxFile --key $KeyFile 2>&1
if ($LASTEXITCODE -ne 0) {
    Write-Host "ERROR al firmar: $signedOutput" -ForegroundColor Red
    exit 1
}

# Save signed tx to file (UTF-8 without BOM)
[IO.File]::WriteAllText((Resolve-Path ".").Path + "\tx-signed.json", $signedOutput)
Write-Host "Firmada -> tx-signed.json" -ForegroundColor Green

# 2. Prepare RPC payload -> temp file
Write-Host "=== Enviando a $RpcUrl ===" -ForegroundColor Cyan

# Read signed JSON, flatten to single line, and escape inner quotes for JSON embedding
$signedRaw = Get-Content "tx-signed.json" -Raw
$signedFlat = $signedRaw -replace "`r`n", "" -replace "`n", ""
$signedEscaped = $signedFlat.Replace('"', '\"')

# Build RPC payload as JSON and write to temp file (UTF-8 without BOM)
$payloadFile = Join-Path $env:TEMP "dsn-send-tx.json"
$rpcBody = @"
{"jsonrpc":"2.0","method":"dsn_sendTransaction","params":{"tx":"$signedEscaped"},"id":1}
"@
[IO.File]::WriteAllText($payloadFile, $rpcBody)

# 3. Submit via curl with @file syntax (most reliable, no shell escaping issues)
try {
    $response = curl.exe -s -X POST $RpcUrl -H "Content-Type: application/json" -d "@$payloadFile" 2>&1
    Remove-Item -Path $payloadFile -Force -ErrorAction SilentlyContinue

    if ($LASTEXITCODE -ne 0) {
        Write-Host "ERROR de conexion: $response" -ForegroundColor Red
        exit 1
    }

    # Parse result
    $result = $response | ConvertFrom-Json
    if ($result.error) {
        Write-Host "ERROR del RPC: $($result.error.message)" -ForegroundColor Red
        exit 1
    }

    Write-Host "`n=== TRANSACCION ENVIADA ===" -ForegroundColor Green
    Write-Host "   Tx Hash: $($result.result)" -ForegroundColor Yellow
    Write-Host "`nPara verificar:" -ForegroundColor Cyan
    Write-Host "   .\dsn.exe contract receipt $($result.result) --rpc $RpcUrl" -ForegroundColor Gray
} catch {
    Write-Host "ERROR: $_" -ForegroundColor Red
    exit 1
}
