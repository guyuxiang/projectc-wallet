package middleware

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/guyuxiang/projectc-custodial-wallet/pkg/config"
	"github.com/guyuxiang/projectc-custodial-wallet/pkg/models"
)

func RequestSignatureMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		cfg := config.GetConfig()
		if cfg == nil || cfg.Signature == nil || cfg.Signature.AppID == "" || cfg.Signature.AppSecret == "" {
			c.Next()
			return
		}

		appID := c.GetHeader("X-App-Id")
		timestamp := c.GetHeader("X-Timestamp")
		signature := c.GetHeader("X-Signature")
		if appID == "" || timestamp == "" || signature == "" {
			c.AbortWithStatusJSON(http.StatusOK, models.Response{Code: models.CodeSignatureInvalid, Message: "missing signature headers", Data: struct{}{}})
			return
		}
		if appID != cfg.Signature.AppID {
			c.AbortWithStatusJSON(http.StatusOK, models.Response{Code: models.CodePermissionDenied, Message: "invalid app id", Data: struct{}{}})
			return
		}

		ts, err := strconv.ParseInt(timestamp, 10, 64)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusOK, models.Response{Code: models.CodeSignatureInvalid, Message: "invalid timestamp", Data: struct{}{}})
			return
		}
		maxSkew := cfg.Signature.MaxSkewMillis
		if maxSkew <= 0 {
			maxSkew = 300000
		}
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

		mac := hmac.New(sha256.New, []byte(cfg.Signature.AppSecret))
		_, _ = mac.Write([]byte(appID + timestamp + string(body)))
		expected := hex.EncodeToString(mac.Sum(nil))
		if !hmac.Equal([]byte(expected), []byte(signature)) {
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
