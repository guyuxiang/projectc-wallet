package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/guyuxiang/projectc-custodial-wallet/pkg/models"
	"github.com/guyuxiang/projectc-custodial-wallet/pkg/service"
)

type SignatureKeyController interface {
	Upsert(c *gin.Context)
}

func NewSignatureKeyController() SignatureKeyController {
	return &signatureKeyController{
		service: service.GetApp().SignatureKey,
	}
}

type signatureKeyController struct {
	service service.SignatureKeyService
}

// Upsert godoc
// @Summary UpsertSignatureKey
// @Description Persist or update a signature key pair by publickeyId.
// @Tags SignatureKey
// @Accept json
// @Produce json
// @Param request body models.SignatureKeyUpsertRequest true "Signature key upsert request"
// @Success 200 {object} models.Response
// @Failure 400 {object} models.Response
// @Failure 401 {object} models.Response
// @Failure 403 {object} models.Response
// @Failure 500 {object} models.Response
// @Router /admin/signature/key/upsert [post]
func (sc *signatureKeyController) Upsert(c *gin.Context) {
	var req models.SignatureKeyUpsertRequest
	if !bindJSON(c, &req) {
		return
	}
	resp, err := sc.service.Upsert(c.Request.Context(), req)
	if err != nil {
		writeAppError(c, err)
		return
	}
	c.JSON(http.StatusOK, models.Response{Code: models.CodeSuccess, Message: "success", Data: resp})
}
