package parser

import (
	"fmt"
	"regexp"
	"testing"
)

func TestParseApis(t *testing.T) {
	p, a, err := ParseApis(
		"/data/apps/go/guess_activity/biz",
		true,
		false)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(p)
	fmt.Println(a)
}

func TestRegExp(t *testing.T) {
	docReg, err := regexp.Compile(`(?si:@api_doc_start(.*?)@api_doc_end)`)
	if nil != err {
		t.Error(err)
		return
	}

	strs := docReg.FindAllStringSubmatch(`
/**
@api_doc_start
	{
		"hello": "world"
	}
@api_doc_end

@api_doc_start
	{
		"hello": "world2"
	}
@api_doc_end

@api_doc_start@api_doc_end

**/
`, -1)

	strJsons := make([]string, 0)
	for _, str := range strs {
		strJsons = append(strJsons, str[1])
	}

	for _, strJson := range strJsons {
		fmt.Println(strJson)
	}
}
