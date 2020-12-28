package parser

import (
	"errors"
	"go/ast"
	"go/token"
	"path/filepath"
	"sort"

	"github.com/haozzzzzzzz/go-rapid-development/utils/uerrors"
	"github.com/haozzzzzzzz/go-tool/api/com/mod"
	"golang.org/x/tools/go/packages"

	"go/types"

	"fmt"

	"strings"

	"github.com/go-playground/validator"
	"github.com/haozzzzzzzz/go-rapid-development/api/request"
	"github.com/sirupsen/logrus"
	"reflect"
	"runtime/debug"
	"time"
)

func (m *ApiParser) ScanApis(
	parseRequestData bool, // 如果parseRequestData会有点慢
	parseCommentText bool, // 是否从注释中提取api。`compile`不能从注释中生成routers
) (
	commonApiParamsMap map[string]*ApiItemParams, // dir -> common params
	apis []*ApiItem,
	err error,
) {
	// scan api dir
	apiDir, err := filepath.Abs(m.ApiDir)
	if nil != err {
		logrus.Warnf("get absolute file apiDir failed. \n%s.", err)
		return
	}

	commonApiParamsMap, apis, err = ParseApis(apiDir, parseRequestData, parseCommentText)
	if nil != err {
		logrus.Errorf("parse apis from code failed. error: %s.", err)
		return
	}

	// merge common params
	if parseRequestData && len(commonApiParamsMap) > 0 {
		for _, api := range apis {
			pkgDir := api.ApiFile.PackageDir
			matchedCommonParams := make([]*ApiItemParams, 0)
			for dir, commonParams := range commonApiParamsMap {
				if strings.Contains(pkgDir, dir) {
					matchedCommonParams = append(matchedCommonParams, commonParams)
				}
			}

			if len(matchedCommonParams) == 0 {
				continue
			}

			for _, commonParams := range matchedCommonParams {
				err = api.ApiItemParams.MergeApiItemParams(commonParams)
				if nil != err {
					logrus.Errorf("api merge common params failed. %s", err)
					return
				}
			}
		}
	}

	// sort api
	sortedApiUriKeys := make([]string, 0)
	mapApi := make(map[string]*ApiItem)
	for _, oneApi := range apis {
		if oneApi.RelativePaths == nil || len(oneApi.RelativePaths) == 0 || oneApi.HttpMethod == "" { // required relative paths and http method
			logrus.Warnf("api need to declare relative paths and http method. func name: %s", oneApi.ApiHandlerFunc)
			continue
		}

		relPath := oneApi.RelativePaths[0] // use first relative paths to sort
		uriKey := m.apiUrlKey(relPath, oneApi.HttpMethod)
		sortedApiUriKeys = append(sortedApiUriKeys, uriKey)
		mapApi[uriKey] = oneApi
	}

	sort.Strings(sortedApiUriKeys)

	sortedApis := make([]*ApiItem, 0)
	for _, key := range sortedApiUriKeys {
		sortedApis = append(sortedApis, mapApi[key])
	}

	apis = sortedApis

	return
}

func ParseApis(
	projectDir string, //项目地址，扫描所有代码
	parseRequestData bool, // 如果parseRequestData会有点慢
	parseCommentText bool, // 是否从注释中提取api。`compile`不能从注释中生成routers
) (
	commonParamsMap map[string]*ApiItemParams, // file dir -> api item params
	apis []*ApiItem,
	err error,
) {
	commonParamsMap = make(map[string]*ApiItemParams)
	apis = make([]*ApiItem, 0)
	fmt.Println("Scan api files ...")
	defer func() {
		if err == nil {
			fmt.Println("Scan api files completed")
		}
	}()

	fmt.Println("loading", projectDir)
	now := time.Now()
	pkgs, err := packages.Load(&packages.Config{
		Mode: packages.NeedName |
			packages.NeedFiles |
			packages.NeedCompiledGoFiles |
			packages.NeedImports |
			packages.NeedDeps |
			packages.NeedExportsFile |
			packages.NeedTypes |
			packages.NeedSyntax |
			packages.NeedTypesInfo |
			packages.NeedTypesSizes,
		Dir:  projectDir,
		Fset: token.NewFileSet(),
	})
	if err != nil {
		panic(err)
	}
	fmt.Println("load", projectDir, time.Now().Sub(now))

	subCommonParamses, subApis, errParse := ParsePkgApis(
		projectDir,
		parseRequestData,
		parseCommentText,
		pkgs,
	)
	err = errParse
	if nil != err {
		logrus.Errorf("parse api file dir %q failed. error: %s.", projectDir, err)
		return
	}

	apis = append(apis, subApis...)

	for _, subCommonParams := range subCommonParamses {
		_, ok := commonParamsMap[projectDir]
		if !ok {
			commonParamsMap[projectDir] = subCommonParams
		} else {
			err = commonParamsMap[projectDir].MergeApiItemParams(subCommonParams)
			if nil != err {
				logrus.Errorf("merge api item common params failed. %#v", subCommonParams)
			}
		}
	}
	return
}

func mergeTypesInfos(info *types.Info, infos ...*types.Info) {
	for _, tempInfo := range infos {
		for tKey, tVal := range tempInfo.Types {
			info.Types[tKey] = tVal
		}

		for defKey, defVal := range tempInfo.Defs {
			info.Defs[defKey] = defVal
		}

		for useKey, useVal := range tempInfo.Uses {
			info.Uses[useKey] = useVal
		}

		for implKey, implVal := range tempInfo.Implicits {
			info.Implicits[implKey] = implVal
		}

		for selKey, selVal := range tempInfo.Selections {
			info.Selections[selKey] = selVal
		}

		for scopeKey, scopeVal := range tempInfo.Scopes {
			info.Scopes[scopeKey] = scopeVal
		}

		// do not need to merge InitOrder
	}
	return
}

func mergeFromProjectPkg(
	projectName string,
	pkg *packages.Package,
	astFiles *[]*ast.File,
	astFileNames map[*ast.File]string,
	typesInfo *types.Info,
) {
	visPkg := map[string]bool{}

	var dfs func(pkg *packages.Package)
	dfs = func(pkg *packages.Package) {
		// 所有文件都过滤的话, 会慢些, 有空看看如何优化
		//if !strings.HasPrefix(pkg.PkgPath, projectName) &&
		//	//TODO: 我自己的包里有些字段需要解析，应当改成过滤条件暴露出去
		//	!strings.HasPrefix(pkg.PkgPath, "zlutils") {
		//	return
		//}
		if visPkg[pkg.PkgPath] {
			return
		}
		visPkg[pkg.PkgPath] = true
		// 合并types.Info
		for _, astFile := range pkg.Syntax {
			if _, ok := astFileNames[astFile]; ok {
				continue
			}
			*astFiles = append(*astFiles, astFile)
			astFileNames[astFile] = pkg.Fset.File(astFile.Pos()).Name()
		}

		// 包内的类型
		mergeTypesInfos(typesInfo, pkg.TypesInfo)
		for _, imp := range pkg.Imports {
			dfs(imp)
		}
	}
	dfs(pkg)
}

func ParsePkgApis(
	apiPackageDir string,
	parseRequestData bool,
	parseCommentText bool, // 是否从注释中提取api。`compile`不能从注释中生成routers
	pkgs []*packages.Package,
) (
	commonParams []*ApiItemParams,
	apis []*ApiItem,
	err error,
) {
	commonParams = make([]*ApiItemParams, 0)
	apis = make([]*ApiItem, 0)
	defer func() {
		if iRec := recover(); iRec != nil {
			err = fmt.Errorf("panic %s. api_dir: %s", iRec, apiPackageDir)
			logrus.WithError(err).Error()
			debug.PrintStack()
		}
	}()

	var goModName, goModDir string
	_, goModName, goModDir = mod.FindGoMod(apiPackageDir)
	if goModName == "" || goModDir == "" {
		err = uerrors.Newf("failed to find go mod")
		return
	}

	// 检索当前目录下所有的文件，不包含import的子文件
	astFiles := make([]*ast.File, 0)
	astFileNames := make(map[*ast.File]string, 0)

	// types
	typesInfo := &types.Info{
		Scopes:     make(map[ast.Node]*types.Scope),
		Defs:       make(map[*ast.Ident]types.Object),
		Uses:       make(map[*ast.Ident]types.Object),
		Types:      make(map[ast.Expr]types.TypeAndValue),
		Implicits:  make(map[ast.Node]types.Object),
		Selections: make(map[*ast.SelectorExpr]*types.Selection),
		InitOrder:  make([]*types.Initializer, 0),
	}

	mergeFromProjectPkg(goModName, pkgs[0], &astFiles, astFileNames, typesInfo)

	for _, astFile := range astFiles { // 遍历当前package的语法树
		fileApis := make([]*ApiItem, 0)
		fileName := astFileNames[astFile]

		// skip *_test.go and routers.go
		if strings.HasSuffix(fileName, "_test.go") || strings.HasSuffix(fileName, "/api/routers.go") {
			logrus.Infof("Skip parsing %s", fileName)
			continue
		}

		fmt.Println("Parsing", fileName)

		// package
		var pkgRelAlias string
		packageName := astFile.Name.Name
		var pkgExportedPath string

		apiFile := NewApiFile()
		apiFile.SourceFile = fileName
		apiFile.PackageName = packageName
		apiFile.PackageExportedPath = pkgExportedPath
		apiFile.PackageRelAlias = pkgRelAlias
		apiFile.PackageDir = fileName

		// search package api types
		for _, decl := range astFile.Decls /*objName, obj := range astFile.Scope.Objects*/ { // 遍历顶层所有变量，寻找HandleFunc
			apiItem := &ApiItem{
				RelativePaths: make([]string, 0),
			}

			funcDecl, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}

			err = ParseBindWrapFuncApi(apiItem, funcDecl, typesInfo, parseRequestData)
			if nil != err {
				logrus.Errorf("parse gin HandlerFunc failed. error: %s.", err)
				return
			}

			fileApis = append(fileApis, apiItem)

		}

		// parse comment text
		if parseCommentText {
			// parse file package comment
			if astFile.Doc != nil {
				pkgComment := astFile.Doc.Text()
				pkgTags, errParse := ParseCommentTags(pkgComment)
				err = errParse
				if nil != err {
					logrus.Errorf("parse pkg comment tags failed. error: %s.", err)
					return
				}

				if pkgTags != nil {
					apiFile.MergeInfoFromCommentTags(pkgTags)
				}

			}

			// scan file comment apis
			for _, commentGroup := range astFile.Comments {
				for _, comment := range commentGroup.List {
					subCommonParams, commentApis, errParse := ParseApisFromPkgCommentText(
						comment.Text,
					)
					err = errParse
					if nil != err {
						logrus.Errorf("parse apis from pkg comment text failed. error: %s.", err)
						return
					}

					fileApis = append(fileApis, commentApis...)

					commonParams = append(commonParams, subCommonParams...)
				}
			}

		}

		for _, api := range fileApis {
			api.ApiFile = apiFile
			api.MergeInfoFromApiInfo()
			apis = append(apis, api)
		}

	}

	return
}

func ParseGinbuilderHandleFuncApi(
	apiItem *ApiItem,
	genDel *ast.GenDecl,
	valueSpec *ast.ValueSpec,
	obj *ast.Ident, // variables with name
	typesInfo *types.Info,
	parseRequestData bool,
) (err error) {
	// ginbuilder.HandlerFunc obj
	apiItem.ApiHandlerFunc = obj.Name
	apiItem.ApiHandlerFuncType = ApiHandlerFuncTypeGinbuilderHandleFunc

	if genDel.Doc != nil {
		apiComment := genDel.Doc.Text()
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

	for _, value := range valueSpec.Values { // 遍历属性
		compositeLit, ok := value.(*ast.CompositeLit)
		if !ok {
			continue
		}

		// compositeLit.Elts
		for _, elt := range compositeLit.Elts {
			keyValueExpr, ok := elt.(*ast.KeyValueExpr)
			if !ok {
				continue
			}

			keyIdent, ok := keyValueExpr.Key.(*ast.Ident)
			if !ok {
				continue
			}

			switch keyIdent.Name {
			case "HttpMethod":
				valueLit, ok := keyValueExpr.Value.(*ast.BasicLit)
				if !ok {
					break
				}

				value := strings.Replace(valueLit.Value, "\"", "", -1)
				value = strings.ToUpper(value)
				switch value {
				case request.METHOD_GET,
					request.METHOD_POST,
					request.METHOD_PUT,
					request.METHOD_PATCH,
					request.METHOD_HEAD,
					request.METHOD_OPTIONS,
					request.METHOD_DELETE,
					request.METHOD_CONNECT,
					request.METHOD_TRACE:
					//request.METHOD_ANY:

				default:
					err = errors.New(fmt.Sprintf("unsupported http method : %s", value))
					logrus.Errorf("mapping unsupported api failed. %s.", err)
					return
				}

				apiItem.HttpMethod = value

			case "RelativePath": // 废弃
				valueLit, ok := keyValueExpr.Value.(*ast.BasicLit)
				if !ok {
					break
				}

				value := strings.Replace(valueLit.Value, "\"", "", -1)
				apiItem.RelativePaths = append(apiItem.RelativePaths, value)

			case "RelativePaths":
				compLit, ok := keyValueExpr.Value.(*ast.CompositeLit)
				if !ok {
					break
				}

				for _, elt := range compLit.Elts {
					basicLit, ok := elt.(*ast.BasicLit)
					if !ok {
						continue
					}

					value := strings.Replace(basicLit.Value, "\"", "", -1)
					apiItem.RelativePaths = append(apiItem.RelativePaths, value)
				}

			case "Handle":
				funcLit, ok := keyValueExpr.Value.(*ast.FuncLit)
				if !ok {
					break
				}

				// if parse request data
				if parseRequestData == false {
					break
				}

				// parse request data
				parseApiFuncBody(apiItem, funcLit.Body, typesInfo)

			}

		}
	}

	err = validator.New().Struct(apiItem)
	if nil != err {
		logrus.Errorf("%#v\n invalid", apiItem)
		return
	}

	return
}

func ParseGinHandlerFuncApi(
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
	parseApiFuncBody(apiItem, funcDecl.Body, typesInfo)

	return

}

func parseApiFuncBody(
	apiItem *ApiItem,
	funcBody *ast.BlockStmt,
	typesInfo *types.Info,
) {
	for _, funcStmt := range funcBody.List {
		switch funcStmt.(type) {
		case *ast.AssignStmt:
			assignStmt := funcStmt.(*ast.AssignStmt)
			lhs := assignStmt.Lhs
			rhs := assignStmt.Rhs

			_ = lhs
			_ = rhs

			for _, expr := range lhs {
				ident, ok := expr.(*ast.Ident)
				if !ok {
					continue
				}

				identType := typesInfo.Defs[ident]
				typeVar, ok := identType.(*types.Var)
				if !ok {
					continue
				}

				switch ident.Name {
				case "headerData":
					iType := parseType(typesInfo, typeVar.Type())
					if iType == nil {
						continue
					}

					structType, ok := iType.(*StructType)
					if ok {
						apiItem.HeaderData = structType
					} else {
						logrus.Warnf("header data is not struct type")
					}

				case "pathData", "uriData":
					iType := parseType(typesInfo, typeVar.Type())
					if iType == nil {
						continue
					}

					structType, ok := iType.(*StructType)
					if ok {
						apiItem.UriData = structType
					} else {
						logrus.Warnf("uri data is not struct type")
					}

				case "queryData":
					iType := parseType(typesInfo, typeVar.Type())
					if iType == nil {
						continue
					}

					structType, ok := iType.(*StructType)
					if ok {
						apiItem.QueryData = structType
					} else {
						logrus.Warnf("query data is not struct type")
					}

				case "postData":
					iType := parseType(typesInfo, typeVar.Type())
					if iType == nil {
						continue
					}

					apiItem.PostData = iType

				case "respData":
					iType := parseType(typesInfo, typeVar.Type())
					if iType == nil {
						continue
					}

					apiItem.RespData = iType

				}
			}

		case *ast.ReturnStmt:

		}

	}

	return
}

/*
对于递归成员，不解析，例如连表节点结构
type Node struct {
	Value int
    Next *Node
}
不解析递归成员Next，简单表示成struct
*/
var parsing = map[types.Type]bool{}

func parseType(
	info *types.Info,
	t types.Type,
) (iType IType) {
	if parsing[t] == true {
		return NewStructType()
	}
	parsing[t] = true
	defer func() { parsing[t] = false }()

	iType = NewBasicType("Unsupported")

	switch t := t.(type) {
	case *types.Basic:
		iType = NewBasicType(t.Name())

	case *types.Pointer:
		iType = parseType(info, t.Elem())

	case *types.Named:
		fmt.Println(t.String())
		if t.String() == "zlutils/time.Time" { //TODO: 目前特殊处理，后续用注释来自定义类型
			iType = NewBasicType("int64")
		} else {
			iType = parseType(info, t.Underlying())

			// 如果是structType
			structType, ok := iType.(*StructType)
			if ok {
				structType.Name = t.Obj().Name()
				iType = structType
			}
		}
	case *types.Struct: // 匿名
		structType := NewStructType()
		typeAstExpr := FindStructAstExprFromInfoTypes(info, t)
		if typeAstExpr == nil { // 找不到expr
			hasFieldsJsonTag := false

			numFields := t.NumFields()
			for i := 0; i <= numFields; i++ {
				mTagParts := parseStringTagParts(t.Tag(i))
				if len(mTagParts) == 0 {
					continue
				}

				if _, ok := mTagParts["json"]; ok {
					hasFieldsJsonTag = true
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

			if tField.Anonymous() {
				fieldType := parseType(info, tField.Type())
				fieldStruct, ok := fieldType.(*StructType)
				if !ok {
					panic(fmt.Errorf("anonymous field:%s must be struct", tField))
				}
				//匿名结构的成员取出来平铺
				if err := structType.AddFields(fieldStruct.Fields...); err != nil {
					logrus.Warnf("parse struct type add field failed. error: %s.", err)
					return
				}
				continue
			}
			//匿名结构即使不导出，也可以被json解析出来
			if !tField.Exported() || parseStringTagParts(t.Tag(i)) == nil {
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

			// tags
			field.Tags = parseStringTagParts(t.Tag(i))

			// definition
			field.Name = tField.Name()
			fieldType := parseType(info, tField.Type())
			field.TypeName = fieldType.TypeName()
			field.TypeSpec = fieldType

			err := structType.AddFields(field)
			if nil != err {
				logrus.Warnf("parse struct type add field failed. error: %s.", err)
				return
			}

		}

		iType = structType

	case *types.Slice:
		arrType := NewArrayType()
		eltType := parseType(info, t.Elem())
		arrType.EltSpec = eltType
		arrType.EltName = eltType.TypeName()
		arrType.Name = fmt.Sprintf("[]%s", eltType.TypeName())

		iType = arrType

	case *types.Array:
		arrType := NewArrayType()
		eltType := parseType(info, t.Elem())
		arrType.Len = t.Len()
		arrType.EltSpec = eltType
		arrType.EltName = eltType.TypeName()
		arrType.Name = fmt.Sprintf("[%d]%s", arrType.Len, eltType.TypeName())

		iType = arrType

	case *types.Map:
		mapType := NewMapType()
		mapType.ValueSpec = parseType(info, t.Elem())
		mapType.KeySpec = parseType(info, t.Key())
		mapType.Name = fmt.Sprintf("map[%s]%s", mapType.KeySpec.TypeName(), mapType.ValueSpec.TypeName())

		iType = mapType

	case *types.Interface:
		iType = NewInterfaceType()

	default:
		logrus.Warnf("parse unsupported type %#v", t)

	}

	return
}

//现在只用得到这两个
var tagKeys = []string{
	"json",
	"binding",
}

//需要能解析 binding:"required,oneof=1 2 3"
func parseStringTagParts(strTag string) (mParts map[string]string) {
	mParts = make(map[string]string, 0)

	tagValue := strings.Replace(strTag, "`", "", -1)
	tag := reflect.StructTag(tagValue)

	for _, k := range tagKeys {
		if v, ok := tag.Lookup(k); ok {
			mParts[k] = v
		}
	}
	return
}

func convertExpr(expr ast.Expr) (newExpr ast.Expr) {
	switch expr.(type) {
	case *ast.StarExpr:
		newExpr = expr.(*ast.StarExpr).X

	case *ast.SelectorExpr:
		newExpr = expr.(*ast.SelectorExpr).Sel

	default:
		newExpr = expr

	}

	return
}

//var stop bool
// struct expr匹配类型
func FindStructAstExprFromInfoTypes(info *types.Info, t *types.Struct) (expr ast.Expr) {
	for tExpr, tType := range info.Types {
		var tStruct *types.Struct
		switch tType.Type.(type) {
		case *types.Struct:
			tStruct = tType.Type.(*types.Struct)
		}

		if tStruct == nil {
			continue
		}

		if t == tStruct {
			// 同一组astFiles生成的Types，内存中对象匹配成功
			expr = tExpr

		} else if t.String() == tStruct.String() {
			// 如果是不同的astFiles生成的Types，可能astFile1中没有这个类型信息，但是另外一组astFiles导入到info里，这是同一个类型，内存对象不一样，但是整体结构是一样的
			// 不是百分百准确
			expr = tExpr

		} else if tStruct.NumFields() == t.NumFields() {
			// 字段一样
			numFields := tStruct.NumFields()
			notMatch := false
			for i := 0; i < numFields; i++ {
				if tStruct.Tag(i) != t.Tag(i) || tStruct.Field(i).Name() != t.Field(i).Name() {
					notMatch = true
					break
				}
			}

			if notMatch == false { // 字段一样
				expr = tExpr
			}
		}

		_, isStructExpr := expr.(*ast.StructType)
		if isStructExpr {
			break
		}
	}

	return
}

func RemoveCommentStartEndToken(text string) (newText string) {
	newText = strings.Replace(text, "//", "", 1)
	newText = strings.Replace(newText, "/*", "", 1)
	newText = strings.Replace(newText, "*/", "", 1)
	newText = strings.TrimSpace(newText)
	return
}
