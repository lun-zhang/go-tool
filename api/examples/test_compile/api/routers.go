package api

import (
	"github.com/gin-gonic/gin"
	request "github.com/haozzzzzzzz/go-tool/api/examples/test_compile/api/request"
)

// 注意：BindRouters函数体内不能自定义添加任何声明，由api compile命令生成api绑定声明
func BindRouters(engine *gin.Engine) (err error) {
	engine.Handle("GET", "/api/test_paths_1", request.TestPaths.GinHandler)
	engine.Handle("GET", "/api/test_paths_2", request.TestPaths.GinHandler)
	engine.Handle("POST", "/test_request", request.TestRequest.GinHandler)
	return
}
