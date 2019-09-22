package parser

import (
	"fmt"
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
		var iType IType
		parseType2(typesInfo, field.Type(), &iType)
		structType, ok := iType.(*StructType)
		if !ok {
			panic("req.Xxx 目前只能是 struct，后期会允许Body不是struct")
		}
		fmt.Println("structType: ", structType)
		apiItem.SetReqData(field.Name(), structType)
	}
}
func parseRespType(
	apiItem *ApiItem,
	typesInfo *types.Info,
	respType types.Type) {
	if respType == nil {
		return
	}

	var iType IType
	parseType2(typesInfo, respType, &iType)
	structType, ok := iType.(*StructType)
	if !ok {
		panic(fmt.Errorf("resp %s 目前只能是 struct，后期会允许不是struct", respType))
	}
	apiItem.RespData = structType
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

//TODO map保存解析过的，不需要重复解析浪费时间
func parseType2(
	info *types.Info,
	t types.Type,
	iType *IType,
) {
	switch t := t.(type) {
	case *types.Basic:
		fmt.Println("Basic: ", t)
		*iType = NewBasicType(t.Name())
	case *types.Pointer:
		fmt.Println("Pointer: ", t)
		parseType2(info, t.Elem(), iType)
	case *types.Named: //这是具名结构体，Underlying为types.Struct才能解析其成员
		fmt.Println("Named: ", t)
		parseType2(info, t.Underlying(), iType)
	case *types.Struct: // 匿名
		fmt.Println("Struct: ", t)
		structType := NewStructType()
		typeAstExpr := FindStructAstExprFromInfoTypes(info, t)
		if typeAstExpr == nil { // 找不到expr
			hasFieldsJsonTag := false

			numFields := t.NumFields()
			for i := 0; i <= numFields; i++ {
				strTag := t.Tag(i)
				mTagParts := parseStringTagParts(strTag)
				if len(mTagParts) == 0 {
					continue
				}

				for key := range mTagParts {
					if key == "json" {
						hasFieldsJsonTag = true
						break
					}
				}

				if hasFieldsJsonTag {
					break
				}
			}

			if hasFieldsJsonTag { // 有导出的jsontag，但是找不到定义的
				logrus.Warnf("cannot found expr of type: %s", t)
			}
		}

		numFields := t.NumFields()
		for i := 0; i < numFields; i++ {
			field := NewField()

			tField := t.Field(i)
			if !tField.Exported() {
				continue
			}

			if typeAstExpr != nil { // 找到声明
				astStructType, ok := typeAstExpr.(*ast.StructType)
				if !ok {
					logrus.Errorf("parse struct type failed. expr: %#v, type: %#v", typeAstExpr, t)
					return
				}

				astField := astStructType.Fields.List[i]

				// 注释
				if astField.Doc != nil && len(astField.Doc.List) > 0 {
					for _, comment := range astField.Doc.List {
						if field.Description != "" {
							field.Description += "; "
						}

						field.Description += RemoveCommentStartEndToken(comment.Text)
					}
				}

				if astField.Comment != nil && len(astField.Comment.List) > 0 {
					for _, comment := range astField.Comment.List {
						if field.Description != "" {
							field.Description += "; "
						}
						field.Description += RemoveCommentStartEndToken(comment.Text)
					}
				}

			}

			if tField.Anonymous() {

			}

			// tags
			field.Tags = parseStringTagParts(t.Tag(i))

			// definition
			field.Name = tField.Name()
			var fieldType IType
			parseType2(info, tField.Type(), &fieldType)
			field.TypeName = fieldType.TypeName()
			field.TypeSpec = fieldType

			err := structType.AddFields(field)
			if nil != err {
				logrus.Warnf("parse struct type add field failed. error: %s.", err)
				return
			}

		}

		*iType = structType

	case *types.Slice:
		arrType := NewArrayType()
		var eltType IType
		parseType2(info, t.Elem(), &eltType)
		arrType.EltSpec = eltType
		arrType.EltName = eltType.TypeName()
		arrType.Name = fmt.Sprintf("[]%s", eltType.TypeName())

		*iType = arrType

	case *types.Map:
		mapType := NewMapType()
		var value IType
		parseType2(info, t.Elem(), &value)
		mapType.ValueSpec = value
		var key IType
		parseType2(info, t.Key(), &key)
		mapType.KeySpec = key
		mapType.Name = fmt.Sprintf("map[%s]%s", mapType.KeySpec.TypeName(), mapType.ValueSpec.TypeName())

		*iType = mapType

	case *types.Interface:
		*iType = NewInterfaceType()

	default:
		logrus.Warnf("parse unsupported type %#v", t)

	}

	return
}
