package api

import (
	"fmt"
	"net/http"
	"runtime"

	"github.com/gin-gonic/gin"

	cfg "github.com/slotopol/server/config"
)

type APIResponse struct {
	Success bool `json:"success"`
	Data    any  `json:"data,omitempty"`
	Error   any  `json:"error,omitempty"`
}

var serverhdr = fmt.Sprintf("slotopol/%s (%s; %s)", cfg.BuildVers, runtime.GOOS, runtime.GOARCH)

func respond(c *gin.Context, code int, success bool, data any, err any) {
	c.Writer.Header().Set("Server", serverhdr)
	c.JSON(code, APIResponse{
		Success: success,
		Data:    data,
		Error:   normalizeError(err),
	})
}

func normalizeError(err any) any {
	if e, ok := err.(error); ok {
		return e.Error()
	}
	return err
}

func RetOk(c *gin.Context, data any) {
	respond(c, http.StatusOK, true, data, nil)
}

func RetErr(c *gin.Context, code int, err any) {
	respond(c, code, false, nil, err)
}

func Ret400(c *gin.Context, err any) {
	RetErr(c, http.StatusBadRequest, err)
}

func Ret401(c *gin.Context, err any) {
	RetErr(c, http.StatusUnauthorized, err)
}

func Ret403(c *gin.Context, err any) {
	RetErr(c, http.StatusForbidden, err)
}

func Ret404(c *gin.Context, err any) {
	RetErr(c, http.StatusNotFound, err)
}

func Ret500(c *gin.Context, err any) {
	RetErr(c, http.StatusInternalServerError, err)
}

func Ret204(c *gin.Context) {
	c.Writer.Header().Set("Server", serverhdr)
	c.Status(http.StatusNoContent)
}
