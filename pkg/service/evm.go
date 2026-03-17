package service

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"strconv"
	"strings"
)

const erc20TransferMethodID = "a9059cbb"

type evmRPCClient struct {
	httpClient *http.Client
	endpoint   string
}

type evmUnsignedTx struct {
	ChainID  string `json:"chainId"`
	Nonce    string `json:"nonce"`
	GasPrice string `json:"gasPrice"`
	GasLimit string `json:"gasLimit"`
	To       string `json:"to"`
	Value    string `json:"value"`
	Data     string `json:"data"`
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

	var result struct {
		Error  interface{}     `json:"error"`
		Result json.RawMessage `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}
	if result.Error != nil {
		return fmt.Errorf("rpc %s failed", method)
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
		GasLimit: formatHexUint64(gasLimit),
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
		GasLimit: formatHexUint64(gasLimit),
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
