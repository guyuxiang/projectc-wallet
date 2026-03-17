package service

import (
	"context"
	"sort"
	"strings"

	"github.com/guyuxiang/projectc-custodial-wallet/pkg/config"
	"github.com/guyuxiang/projectc-custodial-wallet/pkg/models"
	"github.com/guyuxiang/projectc-custodial-wallet/pkg/store"
)

type networkWalletProvider interface {
	NetworkCode() string
	SyncSubscriptions(ctx context.Context) error
	CreateWallet(ctx context.Context, opts walletCreateOptions) (*models.WalletCreateResponse, error)
	QueryWalletInfo(ctx context.Context, wallet *models.WalletEntity, req models.WalletInfoQueryRequest) (*models.WalletInfoQueryResponse, error)
	QueryTransferOutAssets(ctx context.Context, wallet *models.WalletEntity, req models.TransferOutQueryRequest) (*models.TransferOutQueryResponse, error)
	TransferOut(ctx context.Context, wallet *models.WalletEntity, req models.TransferOutRequest) (*models.TransferOutResponse, error)
	HandleTxCallback(ctx context.Context, req models.ConnectorTxCallbackRequest) error
	HandleRollbackCallback(ctx context.Context, req models.ConnectorTxRollbackRequest) error
}

type walletCreateOptions struct {
	WalletNo string
	Network  string
}

func buildNetworkProviders(s *walletService) map[string]networkWalletProvider {
	providers := make(map[string]networkWalletProvider)
	for network, connector := range s.connectors() {
		driver := strings.ToLower(strings.TrimSpace(connector.Driver))
		if driver == "" {
			driver = strings.ToLower(strings.TrimSpace(connector.NetworkCode))
		}
		switch driver {
		case models.NetworkSolana:
			providers[network] = &solanaProvider{svc: s, network: network}
		case models.NetworkEVM:
			providers[network] = &evmProvider{svc: s, network: network}
		}
	}
	return providers
}

func (s *walletService) connectors() map[string]*config.Connector {
	out := make(map[string]*config.Connector)
	if s.cfg == nil {
		return out
	}
	for key, connector := range s.cfg.Connectors {
		network := normalizedNetwork(key)
		if network == "" || connector == nil {
			continue
		}
		normalized := *connector
		if normalized.NetworkCode == "" {
			normalized.NetworkCode = network
		}
		if normalized.Driver == "" {
			normalized.Driver = normalized.NetworkCode
		}
		out[network] = &normalized
	}
	if len(out) == 0 && s.cfg.Connector != nil {
		legacy := *s.cfg.Connector
		network := normalizedNetwork(legacy.NetworkCode)
		if network == "" {
			network = models.NetworkSolana
		}
		if legacy.NetworkCode == "" {
			legacy.NetworkCode = network
		}
		if legacy.Driver == "" {
			legacy.Driver = legacy.NetworkCode
		}
		out[network] = &legacy
	}
	return out
}

func (s *walletService) provider(network string) (networkWalletProvider, error) {
	network = normalizedNetwork(network)
	if network == "" {
		return nil, newAppError(models.CodeNetworkUnsupported, "network not supported")
	}
	provider, ok := s.providers[network]
	if !ok {
		return nil, newAppError(models.CodeNetworkUnsupported, "network not supported")
	}
	return provider, nil
}

func (s *walletService) resolveCreateNetwork(network string) (string, error) {
	networks, err := s.resolveCreateNetworks(network)
	if err != nil {
		return "", err
	}
	return networks[0], nil
}

func (s *walletService) resolveCreateNetworks(network string) ([]string, error) {
	if network := normalizedNetwork(network); network != "" {
		if _, ok := s.providers[network]; ok {
			return []string{network}, nil
		}
		return nil, newAppError(models.CodeNetworkUnsupported, "network not supported")
	}

	networks := make([]string, 0, len(s.providers))
	for network := range s.providers {
		networks = append(networks, network)
	}
	sort.Strings(networks)
	if len(networks) == 0 {
		return nil, newAppError(models.CodeSystemBusy, "no connector is configured")
	}
	return networks, nil
}

func (s *walletService) getWallets(ctx context.Context, walletNo string) ([]models.WalletEntity, error) {
	wallets, err := s.store.GetWalletsByNo(ctx, walletNo)
	if err != nil {
		if store.IsNotFound(err) {
			return nil, newAppError(models.CodeWalletNotFound, "wallet not found")
		}
		return nil, wrapSystemError(err)
	}
	active := make([]models.WalletEntity, 0, len(wallets))
	for _, wallet := range wallets {
		if strings.ToUpper(wallet.Status) != "ACTIVE" {
			continue
		}
		if _, err := s.provider(wallet.Network); err != nil {
			continue
		}
		active = append(active, wallet)
	}
	if len(active) == 0 {
		return nil, newAppError(models.CodeStatusInvalid, "wallet status is not active")
	}
	return active, nil
}

func (s *walletService) getWallet(ctx context.Context, walletNo string, network string) (*models.WalletEntity, error) {
	wallets, err := s.getWallets(ctx, walletNo)
	if err != nil {
		return nil, err
	}
	network = normalizedNetwork(network)
	if network != "" {
		for _, wallet := range wallets {
			if normalizedNetwork(wallet.Network) == network {
				walletCopy := wallet
				return &walletCopy, nil
			}
		}
		return nil, newAppError(models.CodeNetworkUnsupported, "network not supported")
	}
	if len(wallets) == 1 {
		walletCopy := wallets[0]
		return &walletCopy, nil
	}
	return nil, newAppError(models.CodeParamError, "network is required")
}

func (s *walletService) connectorConfig(network string) *config.Connector {
	return s.connectors()[normalizedNetwork(network)]
}

func (s *walletService) connectorPost(ctx context.Context, network string, path string, reqBody interface{}, data interface{}) error {
	baseURL := ""
	username := ""
	password := ""
	if connector := s.connectorConfig(network); connector != nil {
		baseURL = connector.BaseURL
		username = connector.Username
		password = connector.Password
	}
	return s.doJSONRequest(ctx, baseURL, path, username, password, reqBody, data)
}

func (s *walletService) nativeTokenSymbol(network string) string {
	if connector := s.connectorConfig(network); connector != nil && connector.NativeTokenSymbol != "" {
		return connector.NativeTokenSymbol
	}
	if normalizedNetwork(network) == models.NetworkSolana {
		return "SOL"
	}
	return strings.ToUpper(normalizedNetwork(network))
}

func normalizedNetwork(v string) string {
	return strings.ToLower(strings.TrimSpace(v))
}

func newWalletCreateResponse(item models.WalletCreateItem) *models.WalletCreateResponse {
	return &models.WalletCreateResponse{
		WalletNo:   item.WalletNo,
		Network:    item.Network,
		Address:    item.Address,
		KeystoreID: item.KeystoreID,
		Wallets:    []models.WalletCreateItem{item},
	}
}
