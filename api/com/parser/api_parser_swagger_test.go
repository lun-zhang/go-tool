package parser

import (
	"encoding/json"
	"fmt"
	"github.com/go-openapi/spec"
	"testing"
)

func TestSaveApisSwaggerSpec(t *testing.T) {
	swgSpc := NewSwaggerSpec()
	swgSpc.Apis([]*ApiItem{
		{
			ApiItemParams: ApiItemParams{
				HeaderData: &StructType{
					Fields: []*Field{
						{
							Name:     "xx",
							TypeName: "string",
							Tags: map[string]string{
								"json":    "content-type",
								"header":  "content-type",
								"binding": "required",
							},
						},
					},
				},
				UriData: &StructType{
					Fields: []*Field{
						{
							Name:     "tt",
							TypeName: "int64",
							Tags: map[string]string{
								"json":    "book_id",
								"binding": "required",
							},
						},
					},
				},
			},
			Summary: "书本信息接口",
			//PackageName:    "pack",
			ApiHandlerFunc: "func",
			HttpMethod:     "GET",
			RelativePaths: []string{
				"/api/book/:book_id",
				"/api/book",
			},
		},
	})
	swgSpc.Info(
		"Book shop",
		"book shop api for testing tools",
		"1",
		"haozzzzzzzz",
	)
	err := swgSpc.ParseApis()
	if nil != err {
		t.Error(err)
		return
	}

	out, err := swgSpc.Output()
	if nil != err {
		t.Error(err)
		return
	}

	fmt.Println(string(out))
}

func TestSaveApisSwaggerSpec2(t *testing.T) {
	_, a, err := ParseApis(
		//"/data/apps/go/guess_activity/biz",
		"/data/apps/go/vclip_lottery",
		true,
		true)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(a[0])

	swgSpc := NewSwaggerSpec()
	swgSpc.Apis(a)
	swgSpc.Info(
		"vclip抽奖",
		"vclip抽奖接口",
		"1",
		"张伦",
	)
	swgSpc.Schemes([]string{"http"})
	swgSpc.Host("test-m.videobuddy.vid007.com")
	err = swgSpc.ParseApis()
	if nil != err {
		t.Error(err)
		return
	}
	paths := swgSpc.Swagger.Paths.Paths
	for path, item := range paths {
		addComParams(item.Get)
		addComParams(item.Post)
		addComParams(item.Put)
		addComParams(item.Delete)

		paths[path] = item
	}

	out, err := json.Marshal(swgSpc.Swagger)
	if nil != err {
		t.Error(err)
		return
	}

	fmt.Println(string(out))
}

func addComParams(op *spec.Operation) {
	if op == nil {
		return
	}
	for _, tag := range op.Tags {
		switch tag {
		case "app":
			op.AddParam(newParam("header", "User-Id", "string", "用户id，登录才有"))
			op.AddParam(newParam("header", "Device-Id", "string", "设备id"))
			op.AddParam(newParam("header", "Product-Id", "integer", "产品号"))
			op.AddParam(newParam("header", "Version-Code", "integer", "版本号"))
			return
		case "admin":
			op.AddParam(newParam("query", "user_id", "string", "操作者id"))
			op.AddParam(newParam("query", "username", "string", "操作者名字"))
			return
		}
	}
}

func newParam(in, name, typ, desc string) (param *spec.Parameter) {
	param = &spec.Parameter{}
	param.In = in
	param.Name = name
	param.Type = typ
	param.Description = desc
	return
}
