package api

import (
	"github.com/gin-gonic/gin"
	book "github.com/haozzzzzzzz/go-tool/api/examples/test_doc/app/api/book"
)

// 注意：BindRouters函数体内不能自定义添加任何声明，由api compile命令生成api绑定声明
func BindRouters(engine *gin.Engine) (err error) {
	engine.Handle("GET", "/api/book/info/:book_id", book.BookInfo.GinHandler)
	engine.Handle("POST", "/api/book/update/:book_id", book.BookUpdate.GinHandler)
	return
}
