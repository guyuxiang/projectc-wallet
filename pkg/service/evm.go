package service

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strconv"
	"strings"

	"github.com/guyuxiang/projectc-custodial-wallet/pkg/log"
	"golang.org/x/crypto/sha3"
)

const erc20TransferMethodID = "a9059cbb"
const eip7702FactoryFlag = "0x7702"
const entryPointV08Default = "0x4337084D9E255Ff0702461CF8895CE9E3b5Ff108"
const eip7702SingleDefaultMode = "0x0000000000000000000000000000000000000000000000000000000000000000"

type evmUserOperation struct {
	Sender                        string                 `json:"sender"`
	Nonce                         string                 `json:"nonce"`
	Factory                       string                 `json:"factory,omitempty"`
	FactoryData                   string                 `json:"factoryData,omitempty"`
	CallData                      string                 `json:"callData"`
	CallGasLimit                  string                 `json:"callGasLimit"`
	VerificationGasLimit          string                 `json:"verificationGasLimit"`
	PreVerificationGas            string                 `json:"preVerificationGas"`
	MaxPriorityFeePerGas          string                 `json:"maxPriorityFeePerGas"`
	MaxFeePerGas                  string                 `json:"maxFeePerGas"`
	Paymaster                     string                 `json:"paymaster,omitempty"`
	PaymasterVerificationGasLimit string                 `json:"paymasterVerificationGasLimit,omitempty"`
	PaymasterPostOpGasLimit       string                 `json:"paymasterPostOpGasLimit,omitempty"`
	PaymasterData                 string                 `json:"paymasterData,omitempty"`
	EIP7702Auth                   map[string]interface{} `json:"eip7702Auth,omitempty"`
	Signature                     string                 `json:"signature"`
}

type evmUserOperationGasEstimate struct {
	PreVerificationGas            string `json:"preVerificationGas"`
	VerificationGasLimit          string `json:"verificationGasLimit"`
	CallGasLimit                  string `json:"callGasLimit"`
	PaymasterVerificationGasLimit string `json:"paymasterVerificationGasLimit"`
	PaymasterPostOpGasLimit       string `json:"paymasterPostOpGasLimit"`
}

type evmStateOverride map[string]map[string]string

type evmRPCClient struct {
	httpClient *http.Client
	endpoint   string
}

type evmUnsignedTx struct {
	ChainID  string `json:"chainId"`
	Nonce    string `json:"nonce"`
	GasPrice string `json:"gasPrice"`
	Gas      string `json:"gas"`
	To       string `json:"to"`
	Value    string `json:"value"`
	Data     string `json:"data"`
}

type evmCallMsg struct {
	From     string `json:"from,omitempty"`
	To       string `json:"to,omitempty"`
	GasPrice string `json:"gasPrice,omitempty"`
	Value    string `json:"value,omitempty"`
	Data     string `json:"data,omitempty"`
}

func newEVMRPCClient(httpClient *http.Client, endpoint string) *evmRPCClient {
	return &evmRPCClient{httpClient: httpClient, endpoint: strings.TrimSpace(endpoint)}
}

func (c *evmRPCClient) call(ctx context.Context, method string, params []interface{}, out interface{}) error {
	if c == nil || c.endpoint == "" {
		return fmt.Errorf("evm rpc endpoint is not configured")
	}
	reqBody := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  method,
		"params":  params,
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}
	log.Infof("evm rpc request endpoint=%s method=%s payload=%s", c.endpoint, method, truncateLogString(string(body), 2048))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	log.Infof("evm rpc response endpoint=%s method=%s status=%d body=%s", c.endpoint, method, resp.StatusCode, truncateLogString(string(raw), 2048))

	var result struct {
		Error  json.RawMessage `json:"error"`
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return err
	}
	if len(result.Error) > 0 && string(result.Error) != "null" {
		return fmt.Errorf("rpc %s failed: %s", method, strings.TrimSpace(string(result.Error)))
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(result.Result, out)
}

func (c *evmRPCClient) getTransactionCount(ctx context.Context, address string) (uint64, error) {
	var result string
	if err := c.call(ctx, "eth_getTransactionCount", []interface{}{address, "pending"}, &result); err != nil {
		return 0, err
	}
	return parseHexUint64(result)
}

func (c *evmRPCClient) gasPrice(ctx context.Context) (*big.Int, error) {
	var result string
	if err := c.call(ctx, "eth_gasPrice", nil, &result); err != nil {
		return nil, err
	}
	return parseHexBig(result)
}

func (c *evmRPCClient) chainID(ctx context.Context) (uint64, error) {
	var result string
	if err := c.call(ctx, "eth_chainId", nil, &result); err != nil {
		return 0, err
	}
	return parseHexUint64(result)
}

func (c *evmRPCClient) callContract(ctx context.Context, to string, data string) (string, error) {
	var result string
	params := []interface{}{
		map[string]string{
			"to":   strings.TrimSpace(to),
			"data": strings.TrimSpace(data),
		},
		"latest",
	}
	if err := c.call(ctx, "eth_call", params, &result); err != nil {
		return "", err
	}
	return strings.TrimSpace(result), nil
}

func (c *evmRPCClient) callContractWithStateOverride(ctx context.Context, to string, data string, stateOverride evmStateOverride) (string, error) {
	var result string
	params := []interface{}{
		map[string]string{
			"to":   strings.TrimSpace(to),
			"data": strings.TrimSpace(data),
		},
		"latest",
	}
	if len(stateOverride) > 0 {
		params = append(params, stateOverride)
	}
	if err := c.call(ctx, "eth_call", params, &result); err != nil {
		return "", err
	}
	return strings.TrimSpace(result), nil
}

func (c *evmRPCClient) getUserOperationNonce(ctx context.Context, entryPoint string, sender string) (uint64, error) {
	callData, err := buildGetNonceCallData(sender, 0)
	if err != nil {
		return 0, err
	}
	result, err := c.callContract(ctx, entryPoint, callData)
	if err != nil {
		return 0, err
	}
	return parseHexUint64(result)
}

func (c *evmRPCClient) getUserOperationHash(ctx context.Context, entryPoint string, userOp evmUserOperation, stateOverride evmStateOverride) (string, error) {
	callData, err := buildGetUserOpHashCallData(userOp)
	if err != nil {
		return "", err
	}
	overrideJSON, _ := json.Marshal(stateOverride)
	log.Infof("evm getUserOpHash request entryPoint=%s callData=%s stateOverride=%s", entryPoint, truncateLogString(callData, 2048), truncateLogString(string(overrideJSON), 1024))
	result, err := c.callContractWithStateOverride(ctx, entryPoint, callData, stateOverride)
	if err != nil {
		log.Infof("evm getUserOpHash failed entryPoint=%s err=%v", entryPoint, err)
		return "", err
	}
	log.Infof("evm getUserOpHash response entryPoint=%s hash=%s", entryPoint, result)
	raw := normalizeHexBytes(result)
	if len(raw) != 64 {
		return "", fmt.Errorf("invalid getUserOpHash result: %s", result)
	}
	return "0x" + raw, nil
}

func (c *evmRPCClient) getCode(ctx context.Context, address string) (string, error) {
	var result string
	if err := c.call(ctx, "eth_getCode", []interface{}{strings.TrimSpace(address), "latest"}, &result); err != nil {
		return "", err
	}
	return strings.TrimSpace(result), nil
}

func (c *evmRPCClient) estimateUserOperationGas(ctx context.Context, userOp evmUserOperation, entryPoint string, stateOverride evmStateOverride) (*evmUserOperationGasEstimate, error) {
	var result evmUserOperationGasEstimate
	params := []interface{}{userOp, strings.TrimSpace(entryPoint)}
	if len(stateOverride) > 0 {
		params = append(params, stateOverride)
	}
	if err := c.call(ctx, "eth_estimateUserOperationGas", params, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *evmRPCClient) estimateGas(ctx context.Context, msg evmCallMsg) (uint64, error) {
	var result string
	if err := c.call(ctx, "eth_estimateGas", []interface{}{msg}, &result); err != nil {
		return 0, err
	}
	return parseHexUint64(result)
}

func parseHexUint64(value string) (uint64, error) {
	value = strings.TrimSpace(strings.TrimPrefix(value, "0x"))
	if value == "" {
		return 0, nil
	}
	return strconv.ParseUint(value, 16, 64)
}

func parseHexBig(value string) (*big.Int, error) {
	value = strings.TrimSpace(strings.TrimPrefix(value, "0x"))
	if value == "" {
		return big.NewInt(0), nil
	}
	n := new(big.Int)
	if _, ok := n.SetString(value, 16); !ok {
		return nil, fmt.Errorf("invalid hex number")
	}
	return n, nil
}

func buildUnsignedEVMNativeTransferTx(fromAddress string, toAddress string, chainID uint64, nonce uint64, gasPrice *big.Int, gasLimit uint64, amount string) (string, error) {
	valueWei, err := decimalToBaseUnits(amount, 18)
	if err != nil {
		return "", err
	}
	tx := evmUnsignedTx{
		ChainID:  formatHexUint64(chainID),
		Nonce:    formatHexUint64(nonce),
		GasPrice: formatHexBig(gasPrice),
		Gas:      formatHexUint64(gasLimit),
		To:       strings.TrimSpace(toAddress),
		Value:    formatHexBig(valueWei),
		Data:     "0x",
	}
	raw, err := json.Marshal(tx)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func buildUnsignedERC20TransferTx(contractAddress string, toAddress string, chainID uint64, nonce uint64, gasPrice *big.Int, gasLimit uint64, amount string, decimals uint8) (string, error) {
	valueUnits, err := decimalToBaseUnits(amount, decimals)
	if err != nil {
		return "", err
	}
	data, err := buildERC20TransferData(toAddress, valueUnits)
	if err != nil {
		return "", err
	}
	tx := evmUnsignedTx{
		ChainID:  formatHexUint64(chainID),
		Nonce:    formatHexUint64(nonce),
		GasPrice: formatHexBig(gasPrice),
		Gas:      formatHexUint64(gasLimit),
		To:       strings.TrimSpace(contractAddress),
		Value:    "0x0",
		Data:     data,
	}
	raw, err := json.Marshal(tx)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func buildERC20TransferData(toAddress string, amount *big.Int) (string, error) {
	address := strings.TrimPrefix(strings.TrimSpace(toAddress), "0x")
	if len(address) != 40 {
		return "", fmt.Errorf("invalid evm address")
	}
	if _, err := hex.DecodeString(address); err != nil {
		return "", fmt.Errorf("invalid evm address")
	}
	addressWord := strings.Repeat("0", 24) + strings.ToLower(address)
	amountHex := strings.TrimPrefix(amount.Text(16), "0x")
	if amountHex == "" {
		amountHex = "0"
	}
	amountWord := strings.Repeat("0", 64-len(amountHex)) + amountHex
	return "0x" + erc20TransferMethodID + addressWord + amountWord, nil
}

func buildUserOperationTypedData(chainID uint64, entryPoint string, userOp evmUserOperation) (string, error) {
	typedData := map[string]interface{}{
		"types": map[string]interface{}{
			"EIP712Domain": []map[string]string{
				{"name": "name", "type": "string"},
				{"name": "version", "type": "string"},
				{"name": "chainId", "type": "uint256"},
				{"name": "verifyingContract", "type": "address"},
			},
			"PackedUserOperation": []map[string]string{
				{"name": "sender", "type": "address"},
				{"name": "nonce", "type": "uint256"},
				{"name": "initCode", "type": "bytes"},
				{"name": "callData", "type": "bytes"},
				{"name": "accountGasLimits", "type": "bytes32"},
				{"name": "preVerificationGas", "type": "uint256"},
				{"name": "gasFees", "type": "bytes32"},
				{"name": "paymasterAndData", "type": "bytes"},
			},
		},
		"primaryType": "PackedUserOperation",
		"domain": map[string]interface{}{
			"name":              "ERC4337",
			"version":           "0.8",
			"chainId":           chainID,
			"verifyingContract": entryPoint,
		},
		"message": map[string]interface{}{
			"sender":             userOp.Sender,
			"nonce":              userOp.Nonce,
			"initCode":           buildInitCode(userOp.Factory, userOp.FactoryData),
			"callData":           userOp.CallData,
			"accountGasLimits":   packTwoUint128Hex(userOp.VerificationGasLimit, userOp.CallGasLimit),
			"preVerificationGas": userOp.PreVerificationGas,
			"gasFees":            packTwoUint128Hex(userOp.MaxPriorityFeePerGas, userOp.MaxFeePerGas),
			"paymasterAndData":   buildPaymasterAndData(userOp),
		},
	}
	raw, err := json.Marshal(typedData)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func buildUserOperationHash(chainID uint64, entryPoint string, userOp evmUserOperation) (string, error) {
	senderWord, err := abiAddressWord(userOp.Sender)
	if err != nil {
		return "", err
	}
	entryPointWord, err := abiAddressWord(entryPoint)
	if err != nil {
		return "", err
	}
	nonce, err := parseHexBig(userOp.Nonce)
	if err != nil {
		return "", err
	}
	preVerificationGas, err := parseHexBig(userOp.PreVerificationGas)
	if err != nil {
		return "", err
	}

	initCodeHash, err := keccak256Hex(buildUserOperationHashInitCode(userOp))
	if err != nil {
		return "", err
	}
	callDataHash, err := keccak256Hex(userOp.CallData)
	if err != nil {
		return "", err
	}
	paymasterAndDataHash, err := keccak256Hex(buildPaymasterAndData(userOp))
	if err != nil {
		return "", err
	}

	encodedUserOp := senderWord +
		abiUint256Word(nonce) +
		initCodeHash +
		callDataHash +
		strings.TrimPrefix(packTwoUint128Hex(userOp.VerificationGasLimit, userOp.CallGasLimit), "0x") +
		abiUint256Word(preVerificationGas) +
		strings.TrimPrefix(packTwoUint128Hex(userOp.MaxPriorityFeePerGas, userOp.MaxFeePerGas), "0x") +
		paymasterAndDataHash

	userOpHash, err := keccak256Hex("0x" + encodedUserOp)
	if err != nil {
		return "", err
	}

	finalHash, err := keccak256Hex("0x" + userOpHash + entryPointWord + abiUint256Word(new(big.Int).SetUint64(chainID)))
	if err != nil {
		return "", err
	}
	return "0x" + finalHash, nil
}

func buildGetUserOpHashCallData(userOp evmUserOperation) (string, error) {
	senderWord, err := abiAddressWord(userOp.Sender)
	if err != nil {
		return "", err
	}
	nonce, err := parseHexBig(userOp.Nonce)
	if err != nil {
		return "", err
	}
	preVerificationGas, err := parseHexBig(userOp.PreVerificationGas)
	if err != nil {
		return "", err
	}

	initCodeEncoded := abiEncodeBytes(buildPackedInitCode(userOp.Factory, userOp.FactoryData))
	callDataEncoded := abiEncodeBytes(userOp.CallData)
	paymasterAndDataEncoded := abiEncodeBytes(buildPaymasterAndData(userOp))
	signatureEncoded := abiEncodeBytes(userOp.Signature)

	head := make([]string, 0, 9)
	tail := make([]string, 0, 4)
	offset := 32 * 9

	head = append(head, senderWord)
	head = append(head, abiUint256Word(nonce))
	head = append(head, abiUint256Word(big.NewInt(int64(offset))))
	tail = append(tail, initCodeEncoded)
	offset += len(initCodeEncoded) / 2

	head = append(head, abiUint256Word(big.NewInt(int64(offset))))
	tail = append(tail, callDataEncoded)
	offset += len(callDataEncoded) / 2

	head = append(head, strings.TrimPrefix(packTwoUint128Hex(userOp.VerificationGasLimit, userOp.CallGasLimit), "0x"))
	head = append(head, abiUint256Word(preVerificationGas))
	head = append(head, strings.TrimPrefix(packTwoUint128Hex(userOp.MaxPriorityFeePerGas, userOp.MaxFeePerGas), "0x"))

	head = append(head, abiUint256Word(big.NewInt(int64(offset))))
	tail = append(tail, paymasterAndDataEncoded)
	offset += len(paymasterAndDataEncoded) / 2

	head = append(head, abiUint256Word(big.NewInt(int64(offset))))
	tail = append(tail, signatureEncoded)

	return "0x" + methodSelector("getUserOpHash((address,uint256,bytes,bytes,bytes32,uint256,bytes32,bytes,bytes))") + strings.Join(head, "") + strings.Join(tail, ""), nil
}

func buildInitCode(factory string, factoryData string) string {
	factory = strings.TrimSpace(factory)
	factoryData = strings.TrimSpace(factoryData)
	if factory == "" {
		return "0x"
	}
	if factoryData == "" || factoryData == "0x" {
		return factory
	}
	return factory + strings.TrimPrefix(factoryData, "0x")
}

func buildPackedInitCode(factory string, factoryData string) string {
	factory = strings.TrimSpace(factory)
	factoryData = strings.TrimSpace(factoryData)
	if factory == "" {
		return "0x"
	}
	paddedFactory := rightPadHex(factory, 40)
	if factoryData == "" || factoryData == "0x" {
		return "0x" + paddedFactory
	}
	return "0x" + paddedFactory + strings.TrimPrefix(factoryData, "0x")
}

func buildUserOperationHashInitCode(userOp evmUserOperation) string {
	initCode := buildPackedInitCode(userOp.Factory, userOp.FactoryData)
	if !isEIP7702UserOperation(userOp) {
		return initCode
	}
	delegator, _ := userOp.EIP7702Auth["address"].(string)
	delegator = strings.TrimSpace(delegator)
	if !validateEVMAddress(delegator) {
		return initCode
	}
	return "0x" + strings.TrimPrefix(strings.ToLower(delegator), "0x") + strings.TrimPrefix(initCode, "0x")[40:]
}

func buildPaymasterAndData(userOp evmUserOperation) string {
	if strings.TrimSpace(userOp.Paymaster) == "" {
		return "0x"
	}
	parts := []string{userOp.Paymaster}
	if v := strings.TrimSpace(userOp.PaymasterVerificationGasLimit); v != "" {
		parts = append(parts, leftPadHex(strings.TrimPrefix(v, "0x"), 32))
	}
	if v := strings.TrimSpace(userOp.PaymasterPostOpGasLimit); v != "" {
		parts = append(parts, leftPadHex(strings.TrimPrefix(v, "0x"), 32))
	}
	if v := strings.TrimSpace(userOp.PaymasterData); v != "" && v != "0x" {
		parts = append(parts, strings.TrimPrefix(v, "0x"))
	}
	return "0x" + strings.Join(trimHexPrefixes(parts), "")
}

func buildEIP7702ExecuteCallData(target string, value *big.Int, data string) (string, error) {
	targetWord, err := abiAddressWord(target)
	if err != nil {
		return "", err
	}
	valueWord := abiUint256Word(value)
	payload := normalizeHexBytes(data)
	payloadLenWord := abiUint256Word(big.NewInt(int64(len(payload) / 2)))
	paddedPayload := rightPadHex(payload, ((len(payload)+63)/64)*64)

	selector := methodSelector("execute(address,uint256,bytes)")
	offsetWord := leftPadHex("60", 64)
	return "0x" + selector + targetWord + valueWord + offsetWord + payloadLenWord + paddedPayload, nil
}

func buildGetNonceCallData(sender string, key uint64) (string, error) {
	targetWord, err := abiAddressWord(sender)
	if err != nil {
		return "", err
	}
	keyWord := abiUint256Word(new(big.Int).SetUint64(key))
	return "0x" + methodSelector("getNonce(address,uint192)") + targetWord + keyWord, nil
}

func methodSelector(signature string) string {
	hash := sha3.NewLegacyKeccak256()
	hash.Write([]byte(signature))
	return hex.EncodeToString(hash.Sum(nil)[:4])
}

func abiAddressWord(address string) (string, error) {
	address = strings.TrimPrefix(strings.TrimSpace(address), "0x")
	if len(address) != 40 {
		return "", fmt.Errorf("invalid evm address")
	}
	if _, err := hex.DecodeString(address); err != nil {
		return "", fmt.Errorf("invalid evm address")
	}
	return strings.Repeat("0", 24) + strings.ToLower(address), nil
}

func abiUint256Word(v *big.Int) string {
	if v == nil {
		v = big.NewInt(0)
	}
	return leftPadHex(v.Text(16), 64)
}

func packTwoUint128Hex(high string, low string) string {
	return "0x" + leftPadHex(strings.TrimPrefix(strings.TrimSpace(high), "0x"), 32) + leftPadHex(strings.TrimPrefix(strings.TrimSpace(low), "0x"), 32)
}

func isEIP7702UserOperation(userOp evmUserOperation) bool {
	if !strings.EqualFold(strings.TrimSpace(userOp.Factory), eip7702FactoryFlag) {
		return false
	}
	if userOp.EIP7702Auth == nil {
		return false
	}
	_, ok := userOp.EIP7702Auth["address"].(string)
	return ok
}

func abiEncodeBytes(v string) string {
	raw := normalizeHexBytes(v)
	lengthWord := abiUint256Word(big.NewInt(int64(len(raw) / 2)))
	paddedRaw := rightPadHex(raw, ((len(raw)+63)/64)*64)
	return lengthWord + paddedRaw
}

func normalizeHexBytes(v string) string {
	v = strings.TrimPrefix(strings.TrimSpace(v), "0x")
	if len(v)%2 != 0 {
		v = "0" + v
	}
	return strings.ToLower(v)
}

func leftPadHex(v string, length int) string {
	v = normalizeHexBytes(v)
	if len(v) >= length {
		return v
	}
	return strings.Repeat("0", length-len(v)) + v
}

func rightPadHex(v string, length int) string {
	v = normalizeHexBytes(v)
	if len(v) >= length {
		return v
	}
	return v + strings.Repeat("0", length-len(v))
}

func trimHexPrefixes(parts []string) []string {
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		out = append(out, strings.TrimPrefix(strings.TrimSpace(part), "0x"))
	}
	return out
}

func keccak256Hex(v string) (string, error) {
	raw := normalizeHexBytes(v)
	data, err := hex.DecodeString(raw)
	if err != nil {
		return "", err
	}
	hash := sha3.NewLegacyKeccak256()
	hash.Write(data)
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func parseSignatureRSV(signature string) (string, string, uint64, error) {
	raw := normalizeHexBytes(signature)
	if len(raw) != 130 {
		return "", "", 0, fmt.Errorf("invalid signature length")
	}
	r := "0x" + raw[:64]
	s := "0x" + raw[64:128]
	v, err := strconv.ParseUint(raw[128:130], 16, 64)
	if err != nil {
		return "", "", 0, err
	}
	if v >= 27 {
		v -= 27
	}
	return r, s, v, nil
}

func parseEIP7702DelegationIndicator(code string) (string, bool, error) {
	raw := normalizeHexBytes(code)
	if raw == "" || raw == "00" {
		return "", false, nil
	}
	if len(raw) == 46 && strings.HasPrefix(raw, "ef0100") {
		target := "0x" + raw[6:]
		if !validateEVMAddress(target) {
			return "", false, fmt.Errorf("invalid eip7702 delegation indicator")
		}
		return target, true, nil
	}
	return "", false, nil
}

func buildEIP7702DelegationIndicatorCode(delegator string) (string, error) {
	delegator = strings.TrimPrefix(strings.TrimSpace(delegator), "0x")
	if len(delegator) != 40 {
		return "", fmt.Errorf("invalid eip7702 delegator address")
	}
	if _, err := hex.DecodeString(delegator); err != nil {
		return "", fmt.Errorf("invalid eip7702 delegator address")
	}
	return "0xef0100" + strings.ToLower(delegator), nil
}

func decimalToBaseUnits(amount string, decimals uint8) (*big.Int, error) {
	trimmed := strings.TrimSpace(amount)
	if trimmed == "" {
		return nil, fmt.Errorf("invalid amount")
	}
	if strings.HasPrefix(trimmed, "-") {
		return nil, fmt.Errorf("amount must be greater than 0")
	}
	parts := strings.Split(trimmed, ".")
	if len(parts) > 2 {
		return nil, fmt.Errorf("invalid amount")
	}
	intPart := parts[0]
	if intPart == "" {
		intPart = "0"
	}
	fracPart := ""
	if len(parts) == 2 {
		fracPart = parts[1]
	}
	if len(fracPart) > int(decimals) {
		return nil, fmt.Errorf("amount exceeds token decimals")
	}
	for len(fracPart) < int(decimals) {
		fracPart += "0"
	}
	raw := strings.TrimLeft(intPart+fracPart, "0")
	if raw == "" {
		raw = "0"
	}
	value := new(big.Int)
	if _, ok := value.SetString(raw, 10); !ok {
		return nil, fmt.Errorf("invalid amount")
	}
	if value.Sign() <= 0 {
		return nil, fmt.Errorf("amount must be greater than 0")
	}
	return value, nil
}

func formatHexUint64(v uint64) string {
	return fmt.Sprintf("0x%x", v)
}

func formatHexBig(v *big.Int) string {
	if v == nil || v.Sign() == 0 {
		return "0x0"
	}
	return "0x" + v.Text(16)
}
