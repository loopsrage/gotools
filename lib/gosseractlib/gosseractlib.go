package gosseractlib

import "github.com/otiai10/gosseract/v2"

func GosseractClient(tessDataPath, language string) (*gosseract.Client, error) {
	cl := gosseract.NewClient()

	var err error
	err = cl.SetTessdataPrefix(tessDataPath)
	if err != nil {
		return nil, err
	}

	err = cl.SetLanguage(language)
	if err != nil {
		return nil, err
	}

	err = cl.SetVariable("tessedit_ocr_engine_mode", "3")
	if err != nil {
		return nil, err
	}

	return cl, nil
}
