package nodes

const generatedImages = "generatedImages"
const generatedBoxes = "generatedBoxes"
const generatedLtsms = "generatedLtsms"
const generatedGtTxts = "generatedGtTxts"

type GeneratedData int

const (
	Images GeneratedData = iota
	Boxes
	Lstms
	GtTxts
)

func (gd GeneratedData) String() string {
	return [...]string{generatedImages, generatedBoxes, generatedLtsms, generatedGtTxts}[gd]
}

func (gd GeneratedData) Extention() string {
	return [...]string{".tif", ".box", ".lstmf", ".gt.txt"}[gd]
}

func AllGeneratedData() []GeneratedData {
	return []GeneratedData{Images, Boxes, Lstms, GtTxts}
}

type RequiredNode int

const (
	Text2ImageNode RequiredNode = iota
	GenerateLineBoxNode
	Tesseract13LstmTrainNode
	ReadAndCompressFilesNode
	AggregateStatsNode
)

func (gd RequiredNode) String() string {
	return [...]string{"Text2Image", "GenerateLineBox", "Tesseract13Lstm", "ReadAndCompress", "AggregateStats"}[gd]
}

func AllRequiredNodes() []RequiredNode {
	return []RequiredNode{Text2ImageNode, GenerateLineBoxNode, Tesseract13LstmTrainNode, ReadAndCompressFilesNode, AggregateStatsNode}
}
