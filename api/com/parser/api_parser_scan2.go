package parser

import (
	"github.com/sirupsen/logrus"
	"go/ast"
	"go/types"
)

func ParseBindWrapFuncApi(
	apiItem *ApiItem,
	funcDecl *ast.FuncDecl,
	typesInfo *types.Info,
	parseRequestData bool,
) (err error) {
	apiItem.ApiHandlerFunc = funcDecl.Name.Name
	apiItem.ApiHandlerFuncType = ApiHandlerFuncTypeGinHandlerFunc

	// 读取注释
	if funcDecl.Doc != nil {
		apiComment := funcDecl.Doc.Text()
		commentTags, errParse := ParseCommentTags(apiComment)
		err = errParse
		if nil != err {
			logrus.Errorf("parse api comment tags failed. error: %s.", err)
			return
		}

		if commentTags != nil {
			apiItem.MergeInfoFomCommentTags(commentTags)
		}
	}

	if !parseRequestData {
		return
	}

	// parse request data
	parseBindWrapFunc(apiItem, typesInfo)

	return

}
func parseBindWrapFunc(
	apiItem *ApiItem,
	typesInfo *types.Info,
) {

	for _, obj := range typesInfo.Defs {
		if obj == nil {
			continue
		}
		function, ok := obj.Type().(*types.Signature)
		if !ok {
			continue
		}
		reqType, ok := checkIn(function.Params())
		if !ok {
			continue
		}
		respType, ok := checkOut(function.Results())
		if !ok {
			continue
		}
		//fmt.Println(obj.Pkg())
		parseReqType(apiItem, typesInfo, reqType)
		parseRespType(apiItem, typesInfo, respType)
	}
	return
}

func parseReqType(
	apiItem *ApiItem,
	typesInfo *types.Info,
	reqType *types.Struct) {
	if reqType == nil {
		return
	}
	for i := 0; i < reqType.NumFields(); i++ {
		field := reqType.Field(i)

		if field.Name() == "Meta" {
			continue
		}
		iType := parseType(typesInfo, field.Type())
		apiItem.SetReqData(field.Name(), iType)
	}
}
func parseRespType(
	apiItem *ApiItem,
	typesInfo *types.Info,
	respType types.Type) {
	if respType == nil {
		return
	}

	iType := parseType(typesInfo, respType)
	apiItem.RespData = iType
}

func checkIn(params *types.Tuple) (reqType *types.Struct, ok bool) {
	if params.Len() <= 0 || params.Len() > 2 {
		return nil, false
	}
	ctxType := params.At(0)
	if ctxType.Type().String() != "context.Context" {
		return nil, false
	}
	if params.Len() == 1 {
		return nil, true
	}
	reqType, ok = params.At(1).Type().(*types.Struct)
	return
}
func checkOut(results *types.Tuple) (respType types.Type, ok bool) {
	switch results.Len() {
	case 0:
		return nil, true
	case 1: //resp or err
		out0Type := results.At(0).Type()
		if out0Type.String() == "error" {
			return nil, true
		}
		if results.At(0).Name() != "resp" {
			return nil, false
		}
		return out0Type, true
	case 2:
		if results.At(0).Name() != "resp" {
			return nil, false
		}
		errType := results.At(1)
		if errType.Type().String() != "error" {
			return nil, false
		}
		return results.At(0).Type(), true
	default:
		return nil, false
	}
}
