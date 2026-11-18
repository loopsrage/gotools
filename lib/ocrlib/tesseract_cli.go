package ocrlib

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// TesseractCLIProvider executes Tesseract LSTM OCR over stdin/stdout TSV without temp files.
type TesseractCLIProvider struct {
	binPath     string
	tessdataDir string
	language    string
	psm         int
}

// NewTesseractCLIProvider locates the tesseract executable and constructs a TesseractCLIProvider.
func NewTesseractCLIProvider(cfg TesseractProviderConfig) (*TesseractCLIProvider, error) {
	bin := strings.TrimSpace(cfg.BinaryPath)
	if bin == "" {
		if found, err := exec.LookPath("tesseract"); err == nil {
			bin = found
		} else {
			for _, candidate := range []string{"/opt/homebrew/bin/tesseract", "/usr/local/bin/tesseract", "/usr/bin/tesseract"} {
				if info, statErr := os.Stat(candidate); statErr == nil && !info.IsDir() {
					bin = candidate
					break
				}
			}
		}
	}
	if bin == "" {
		return nil, errors.New("ocrlib(tesseract): tesseract executable not found")
	}
	lang := strings.TrimSpace(cfg.Language)
	if lang == "" {
		lang = "eng"
	}
	psm := cfg.PSM
	if psm <= 0 {
		psm = 6
	}
	return &TesseractCLIProvider{
		binPath:     bin,
		tessdataDir: strings.TrimSpace(cfg.TessdataDir),
		language:    lang,
		psm:         psm,
	}, nil
}

func (t *TesseractCLIProvider) Name() string    { return "tesseract" }
func (t *TesseractCLIProvider) ModelID() string { return "tesseract-lstm:" + t.language }
func (t *TesseractCLIProvider) Close() error    { return nil }

func (t *TesseractCLIProvider) Recognize(ctx context.Context, req RecognizeRequest) (*RecognizeResult, error) {
	if len(req.ImagePNG) == 0 {
		return nil, errors.New("ocrlib(tesseract): empty ImagePNG")
	}
	start := time.Now()
	lang := req.Language
	if strings.TrimSpace(lang) == "" {
		lang = t.language
	}
	psm := t.psm
	if req.Params != nil {
		if rawPSM, ok := req.Params["psm"]; ok {
			switch v := rawPSM.(type) {
			case int:
				psm = v
			case float64:
				psm = int(v)
			}
		}
	}

	args := []string{"stdin", "stdout", "-l", lang, "--psm", strconv.Itoa(psm)}
	if t.tessdataDir != "" {
		args = append(args, "--tessdata-dir", t.tessdataDir)
	}
	args = append(args, "tsv")

	cmd := exec.CommandContext(ctx, t.binPath, args...)
	cmd.Stdin = bytes.NewReader(req.ImagePNG)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ocrlib(tesseract): %w (%s)", err, strings.TrimSpace(stderr.String()))
	}

	text, tokens, meanConf := parseTesseractTSV(stdout.String())
	return &RecognizeResult{
		Provider:   t.Name(),
		ModelID:    "tesseract-lstm:" + lang,
		Text:       text,
		Confidence: meanConf,
		Tokens:     tokens,
		LatencyMs:  float64(time.Since(start).Microseconds()) / 1000.0,
		Metadata: map[string]any{
			"psm":      psm,
			"language": lang,
		},
	}, nil
}

func parseTesseractTSV(tsv string) (string, []TokenBox, float64) {
	lines := strings.Split(tsv, "\n")
	var (
		tokens    []TokenBox
		confSum   float64
		confCount int
		outLines  []string
		currWords []string
		prevLineK string
	)

	for i, line := range lines {
		if i == 0 || strings.TrimSpace(line) == "" {
			continue
		}
		cols := strings.Split(line, "\t")
		if len(cols) < 12 {
			continue
		}
		level, _ := strconv.Atoi(cols[0])
		if level != 5 {
			continue
		}
		word := strings.TrimSpace(cols[11])
		if word == "" {
			continue
		}
		left, _ := strconv.Atoi(cols[6])
		top, _ := strconv.Atoi(cols[7])
		width, _ := strconv.Atoi(cols[8])
		height, _ := strconv.Atoi(cols[9])
		conf, err := strconv.ParseFloat(cols[10], 64)

		var normConf float64
		if err == nil && conf >= 0 {
			normConf = conf / 100.0
			confSum += normConf
			confCount++
		}

		lineKey := cols[1] + ":" + cols[2] + ":" + cols[3] + ":" + cols[4]
		if prevLineK != "" && lineKey != prevLineK && len(currWords) > 0 {
			outLines = append(outLines, strings.Join(currWords, " "))
			currWords = currWords[:0]
		}
		prevLineK = lineKey
		currWords = append(currWords, word)

		tokens = append(tokens, TokenBox{
			Text:       word,
			Confidence: normConf,
			Box:        image.Rect(left, top, left+width, top+height),
		})
	}
	if len(currWords) > 0 {
		outLines = append(outLines, strings.Join(currWords, " "))
	}

	var mean float64
	if confCount > 0 {
		mean = confSum / float64(confCount)
	}
	return strings.TrimSpace(strings.Join(outLines, "\n")), tokens, mean
}
