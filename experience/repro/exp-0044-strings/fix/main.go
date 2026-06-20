package main

import (
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

func main() {
	_ = cases.Title(language.English).String("hello world")
}
