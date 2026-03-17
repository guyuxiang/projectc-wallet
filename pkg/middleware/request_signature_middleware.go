package middleware

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/guyuxiang/projectc-custodial-wallet/pkg/models"
	"github.com/guyuxiang/projectc-custodial-wallet/pkg/service"
	"github.com/guyuxiang/projectc-custodial-wallet/pkg/signature"
)

func RequestSignatureMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		app := service.GetApp()
		if app == nil || app.SignatureKey == nil {
			c.AbortWithStatusJSON(http.StatusOK, models.Response{Code: models.CodeSystemBusy, Message: "signature service is unavailable", Data: struct{}{}})
			return
		}
		publickeyID := c.GetHeader("X-Publickey-ID")
		timestamp := c.GetHeader("X-Timestamp")
		signatureValue := c.GetHeader("X-Signature")
		if publickeyID == "" || timestamp == "" || signatureValue == "" {
			c.AbortWithStatusJSON(http.StatusOK, models.Response{Code: models.CodeSignatureInvalid, Message: "missing signature headers", Data: struct{}{}})
			return
		}
		key, err := app.SignatureKey.GetKeyByID(context.Background(), publickeyID)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusOK, models.Response{Code: models.CodeSystemBusy, Message: err.Error(), Data: struct{}{}})
			return
		}
		if key == nil || key.PublicKey == "" {
			c.AbortWithStatusJSON(http.StatusOK, models.Response{Code: models.CodePermissionDenied, Message: "invalid publickey id", Data: struct{}{}})
			return
		}

		ts, err := strconv.ParseInt(timestamp, 10, 64)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusOK, models.Response{Code: models.CodeSignatureInvalid, Message: "invalid timestamp", Data: struct{}{}})
			return
		}
		maxSkew := int64(300000)
		if absDuration(time.Now().UnixMilli()-ts) > maxSkew {
			c.AbortWithStatusJSON(http.StatusOK, models.Response{Code: models.CodeSignatureInvalid, Message: "timestamp expired", Data: struct{}{}})
			return
		}

		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusOK, models.Response{Code: models.CodeSystemBusy, Message: err.Error(), Data: struct{}{}})
			return
		}
		c.Request.Body = io.NopCloser(bytes.NewReader(body))

		if err := signature.VerifyBase64(
			key.PublicKey,
			signatureValue,
			c.Request.Method,
			c.Request.URL.Path,
			c.Request.URL.RawQuery,
			body,
			timestamp,
		); err != nil {
			c.AbortWithStatusJSON(http.StatusOK, models.Response{Code: models.CodeSignatureInvalid, Message: "invalid signature", Data: struct{}{}})
			return
		}
		c.Next()
	}
}

func absDuration(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}
