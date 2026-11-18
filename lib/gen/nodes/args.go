package nodes

import (
	"fmt"
	"strings"
)

type Args struct {
	GeneratedAsin string
	OutputBase    string
	FontName      string
	FontDir       string
	Initial       string
	InitialTif    string
	InitialGtTxt  string
}

func (a *Args) IterationName(der string) string {
	return strings.TrimRight(fmt.Sprintf("%s_%s", a.Initial, der), "_")
}

func (a *Args) IterationTif(der string) string {
	return a.IterationName(der) + Images.Extention()
}

func (a *Args) IterationGtTxt(der string) string {
	return a.IterationName(der) + GtTxts.Extention()
}

func (a *Args) IterationBox(der string) string {
	return a.IterationName(der) + Boxes.Extention()
}

func (a *Args) IterationLtsm(der string) string {
	return a.IterationName(der) + Lstms.Extention()
}
