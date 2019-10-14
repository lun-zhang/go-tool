package parser

import (
	"fmt"
	"github.com/sirupsen/logrus"
	"go/ast"
	"go/types"
	"runtime"
	"time"
)

func Caller(skip int) (name string) {
	name = "unknown"
	if pc, _, line, ok := runtime.Caller(skip); ok {
		name = fmt.Sprintf("%s:%d", runtime.FuncForPC(pc).Name(), line)
	}
	return
}

func howLong() func() {
	now := time.Now()
	return func() {
		d := time.Now().Sub(now)
		if d > time.Millisecond*500 {
			fmt.Println(Caller(2), d)
		}
	}
}

func ParseBindWrapFuncApi(
	apiItem *ApiItem,
	funcDecl *ast.FuncDecl,
	typesInfo *types.Info,
	parseRequestData bool,
) (err error) {
	defer howLong()()
	apiItem.ApiHandlerFunc = funcDecl.Name.Name
	apiItem.ApiHandlerFuncType = ApiHandlerFuncTypeGinHandlerFunc

	// 读取注释
	if funcDecl.Doc == nil {
		return
	}
	apiComment := funcDecl.Doc.Text()
	commentTags, err := ParseCommentTags(apiComment)
	if nil != err {
		logrus.Errorf("parse api comment tags failed. error: %s.", err)
		return
	}

	if commentTags == nil {
		return
	}
	//没正确注释的不解析
	if commentTags.Summary == "" ||
		commentTags.LineTagDocHttpMethod == "" ||
		len(commentTags.LineTagDocRelativePaths) == 0 {
		return
	}
	apiItem.MergeInfoFomCommentTags(commentTags)

	if !parseRequestData {
		return
	}

	fmt.Println("parse func: ", funcDecl.Name)
	// parse request data
	if !checkInAst(funcDecl.Type.Params) {
		return
	}
	if !checkOutAst(funcDecl.Type.Results) {
		return
	}
	parseBindWrapFunc(apiItem, funcDecl.Type, typesInfo)

	return

}
func parseBindWrapFunc(
	apiItem *ApiItem,
	funcType *ast.FuncType,
	typesInfo *types.Info,
) {

	parseReqType(apiItem, typesInfo, funcType.Params)
	parseRespType(apiItem, typesInfo, funcType.Results)
	return
}

func parseReqType(
	apiItem *ApiItem,
	typesInfo *types.Info,
	params *ast.FieldList) {
	if params == nil {
		return
	}
	n := len(params.List)
	if n == 1 {
		return
	}
	reqType := params.List[1].Type.(*ast.StructType)

	for i := 0; i < len(reqType.Fields.List); i++ {
		field := reqType.Fields.List[i]
		expr := field.Type

		//identType := typesInfo.Defs[]
		//typeVar:= identType.(*types.Var)
		iType := parseType(typesInfo, typesInfo.Types[expr].Type)
		apiItem.SetReqData(field.Names[0].Name, iType)
	}
}

func parseRespType(
	apiItem *ApiItem,
	typesInfo *types.Info,
	results *ast.FieldList) {
	if results == nil {
		return
	}
	var respType ast.Expr
	switch len(results.List) {
	case 1:
		out0Type := results.List[0].Type
		if exprIsErrorType(out0Type) {
			return
		}
		respType = out0Type
	case 2:
		respType = results.List[0].Type
	default:
		return
	}
	iType := parseType(typesInfo, typesInfo.Types[respType].Type)
	apiItem.RespData = iType
}

func checkInAst(params *ast.FieldList) bool {
	if params == nil {
		return false
	}
	n := len(params.List)

	if n <= 0 { //n > 2在switch判断
		return false
	}

	{ //第一个参数必须是context.Context类型
		ctxType, ok := params.List[0].Type.(*ast.SelectorExpr)
		if !ok {
			return false
		}
		c, ok := ctxType.X.(*ast.Ident)
		if !ok {
			return false
		}
		if c.Name != "context" || ctxType.Sel.Name != "Context" {
			return false
		}
	}
	switch n {
	case 1:
		return true
	case 2:
		req, ok := parseAstTypeToStruct(params.List[1].Type)
		if !ok {
			return false
		}
		for i := 0; i < len(req.Fields.List); i++ {
			field := req.Fields.List[i]
			switch field.Names[0].Name {
			case "Body":
			case "Query":
			case "Header":
			case "Uri":
			case "Meta":
			default: //包含不能识别的类型
				return false
			}
		}
		return true
	default: //n > 2
		return false
	}
}

func checkOutAst(results *ast.FieldList) bool {
	if results == nil {
		return true
	}
	switch len(results.List) {
	case 0:
		return true
	case 1:
		return true
	case 2:
		errType := results.List[1].Type
		if !exprIsErrorType(errType) {
			return false
		}
		return true
	default:
		return false
	}
}

func exprIsErrorType(expr ast.Expr) bool {
	i, ok := expr.(*ast.Ident)
	if !ok {
		return false
	}
	if i.Name != "error" {
		return false
	}
	return true
}

func parseAstTypeToStruct(t ast.Expr) (*ast.StructType, bool) {
	if t == nil {
		return nil, false
	}
	switch t := t.(type) {
	case *ast.StructType:
		return t, true
	case *ast.Ident:
		return parseAstTypeToStruct(t.Obj.Decl.(*ast.TypeSpec).Type)
	default:
		return nil, false
	}
}
