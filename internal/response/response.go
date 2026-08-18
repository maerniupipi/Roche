package response

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// 统一响应结构
// code=0 表示成功，非0表示错误
// msg  响应消息
// data 响应数据

type R struct {
	Code int         `json:"code"`
	Msg  string      `json:"msg"`
	Data interface{} `json:"data"`
}

func Success(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, R{Code: 0, Msg: "success", Data: data})
}

func SuccessWithMsg(c *gin.Context, msg string, data interface{}) {
	c.JSON(http.StatusOK, R{Code: 0, Msg: msg, Data: data})
}

func Created(c *gin.Context, data interface{}) {
	c.JSON(http.StatusCreated, R{Code: 0, Msg: "success", Data: data})
}

func Page(c *gin.Context, list interface{}, total int64, page int, pageSize int) {
	c.JSON(http.StatusOK, R{
		Code: 0,
		Msg:  "success",
		Data: gin.H{
			"list":      list,
			"total":     total,
			"page":      page,
			"page_size": pageSize,
		},
	})
}

func NoContent(c *gin.Context) {
	c.JSON(http.StatusNoContent, R{Code: 0, Msg: "success", Data: nil})
}

// 错误响应

func Error(c *gin.Context, httpStatus int, code int, msg string) {
	c.JSON(httpStatus, R{Code: code, Msg: msg, Data: nil})
}

func BadRequest(c *gin.Context, msg string) {
	c.JSON(http.StatusBadRequest, R{Code: http.StatusBadRequest, Msg: msg, Data: nil})
}

func Unauthorized(c *gin.Context, msg string) {
	c.JSON(http.StatusUnauthorized, R{Code: http.StatusUnauthorized, Msg: msg, Data: nil})
}

func Forbidden(c *gin.Context, msg string) {
	c.JSON(http.StatusForbidden, R{Code: http.StatusForbidden, Msg: msg, Data: nil})
}

func NotFound(c *gin.Context, msg string) {
	c.JSON(http.StatusNotFound, R{Code: http.StatusNotFound, Msg: msg, Data: nil})
}

func Conflict(c *gin.Context, msg string) {
	c.JSON(http.StatusConflict, R{Code: http.StatusConflict, Msg: msg, Data: nil})
}

func InternalError(c *gin.Context, msg string) {
	msg2 := msg
	if msg2 == "" {
		msg2 = "Internal server error"
	}
	c.JSON(http.StatusInternalServerError, R{Code: http.StatusInternalServerError, Msg: msg2, Data: nil})
}
