package controller

import (
	"github.com/gin-gonic/gin"
	"github.com/guyuxiang/projectc-custodial-wallet/pkg/models"
	"github.com/guyuxiang/projectc-custodial-wallet/pkg/service"
)

type ToDoController interface {
	GetToDo(c *gin.Context)
}

func NewToDoController() ToDoController {
	return &toDoController{
		toDoService: service.NewToDoService(),
	}
}

type toDoController struct {
	toDoService service.ToDoService
}

// GetToDo godoc
// @Summary GetToDo
// @Description Get todo demo response.
// @Tags ToDo
// @Produce json
// @Success 200 {object} models.Response
// @Failure 400 {object} models.Response
// @Failure 401 {object} models.Response
// @Failure 403 {object} models.Response
// @Failure 500 {object} models.Response
// @Router /todo/get [get]
func (this *toDoController) GetToDo(c *gin.Context) {
	this.toDoService.Get()
	c.JSON(200, models.Response{Code: "0", Message: "todo demo", Data: struct{}{}})
	return
}
