package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/guyuxiang/projectc-custodial-wallet/pkg/models"
	"github.com/guyuxiang/projectc-custodial-wallet/pkg/store"
)

type SignatureKeyMaterial struct {
	PublicKey  string
	PrivateKey string
}

type SignatureKeyService interface {
	Upsert(ctx context.Context, req models.SignatureKeyUpsertRequest) (*models.SignatureKeyUpsertResponse, error)
	GetKeyByID(ctx context.Context, publickeyID string) (*SignatureKeyMaterial, error)
	DefaultKey() (string, *SignatureKeyMaterial, error)
}

type signatureKeyService struct {
	store store.Store
}

func NewSignatureKeyService(st store.Store) SignatureKeyService {
	return &signatureKeyService{store: st}
}

func (s *signatureKeyService) Upsert(ctx context.Context, req models.SignatureKeyUpsertRequest) (*models.SignatureKeyUpsertResponse, error) {
	key := &models.SignatureKeyEntity{
		PublickeyID: strings.TrimSpace(req.PublickeyID),
		PublicKey:   strings.TrimSpace(req.PublicKey),
		PrivateKey:  strings.TrimSpace(req.PrivateKey),
	}
	if key.PublickeyID == "" || key.PublicKey == "" || key.PrivateKey == "" {
		return nil, newAppError(models.CodeParamError, "publickeyId, publicKey and privateKey are required")
	}
	if err := s.store.UpsertSignatureKey(ctx, key); err != nil {
		return nil, wrapSystemError(err)
	}
	return &models.SignatureKeyUpsertResponse{PublickeyID: key.PublickeyID}, nil
}

func (s *signatureKeyService) GetKeyByID(ctx context.Context, publickeyID string) (*SignatureKeyMaterial, error) {
	publickeyID = strings.TrimSpace(publickeyID)
	if publickeyID == "" {
		return nil, nil
	}
	key, err := s.store.GetSignatureKeyByID(ctx, publickeyID)
	if err == nil {
		return &SignatureKeyMaterial{
			PublicKey:  key.PublicKey,
			PrivateKey: key.PrivateKey,
		}, nil
	}
	if err != nil && !store.IsNotFound(err) {
		return nil, wrapSystemError(err)
	}
	return nil, nil
}

func (s *signatureKeyService) DefaultKey() (string, *SignatureKeyMaterial, error) {
	keys, err := s.store.ListSignatureKeys(context.Background())
	if err != nil {
		return "", nil, wrapSystemError(err)
	}
	if len(keys) == 0 {
		return "", nil, nil
	}
	for _, key := range keys {
		if strings.EqualFold(key.PublickeyID, "default") {
			return key.PublickeyID, &SignatureKeyMaterial{
				PublicKey:  key.PublicKey,
				PrivateKey: key.PrivateKey,
			}, nil
		}
	}
	if len(keys) == 1 {
		key := keys[0]
		return key.PublickeyID, &SignatureKeyMaterial{
			PublicKey:  key.PublicKey,
			PrivateKey: key.PrivateKey,
		}, nil
	}
	return "", nil, newAppError(models.CodeSystemBusy, fmt.Sprintf("multiple signature keys exist, default key is ambiguous"))
}
