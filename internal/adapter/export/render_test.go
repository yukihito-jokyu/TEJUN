package export

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"regexp"
	"strings"
	"testing"
)

func TestRender(t *testing.T) {
	document := []byte(
		`{"title":"日本語の手順書","overview":"概要","prerequisites":["準備"],"steps":[{"title":"開始","description":"説明","command":"echo ok","notes":["注意"],"evidenceRefs":[]}]}`,
	)

	tests := []struct {
		name, format, prefix string
	}{
		{name: "markdown", format: "markdown", prefix: "# 日本語の手順書"},
		{name: "pdf", format: "pdf", prefix: "%PDF-"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			if err := Render(test.format, document, &output); err != nil {
				t.Fatal(err)
			}

			if !strings.HasPrefix(output.String(), test.prefix) {
				t.Fatalf("output lacks %q", test.prefix)
			}
		})
	}
}

func TestPDFLongContentHasPages(t *testing.T) {
	document := fmt.Sprintf(
		`{"title":"長文の手順書","steps":[{"title":"開始","description":%q,"command":%q,"notes":["注意"]}]}`,
		strings.Repeat("長い説明です。", 1000),
		strings.Repeat("echo hello world\n", 100),
	)

	var output bytes.Buffer
	if err := Render("pdf", []byte(document), &output); err != nil {
		t.Fatal(err)
	}

	if pages := len(regexp.MustCompile(`/Type\s*/Page\b`).FindAll(output.Bytes(), -1)); pages < 2 {
		t.Fatalf("page count = %d, want at least 2", pages)
	}
}

func TestPDFEvidenceImage(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 40, 20))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})

	var encoded bytes.Buffer
	if err := png.Encode(&encoded, img); err != nil {
		t.Fatal(err)
	}

	document := []byte(
		`{"title":"手順","steps":[{"title":"撮影","evidenceRefs":[{"evidenceId":"evidence","displayName":"画面","included":true}]}]}`,
	)

	tests := []struct {
		name    string
		read    func(string, string) ([]byte, error)
		wantErr bool
	}{
		{"available", func(procedure, evidence string) ([]byte, error) {
			if procedure != "procedure" || evidence != "evidence" {
				t.Fatal("wrong relation")
			}

			return encoded.Bytes(), nil
		}, false},
		{"missing", nil, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer

			err := RenderWithEvidence("pdf", document, "procedure", test.read, &output)
			if (err != nil) != test.wantErr {
				t.Fatalf("error = %v", err)
			}

			if !test.wantErr && !bytes.Contains(output.Bytes(), []byte("/Subtype /Image")) {
				t.Fatal("PDF does not contain an image")
			}
		})
	}
}

func TestJPEGExifOrientation(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 40, 20))

	var jpegData bytes.Buffer
	if err := jpeg.Encode(&jpegData, img, nil); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name        string
		orientation uint16
		wantWidth   float64
		wantHeight  float64
	}{
		{"normal", 1, 40, 20},
		{"rotated", 6, 20, 40},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tiff := make([]byte, 26)
			copy(tiff, []byte("II"))
			binary.LittleEndian.PutUint16(tiff[2:], 42)
			binary.LittleEndian.PutUint32(tiff[4:], 8)
			binary.LittleEndian.PutUint16(tiff[8:], 1)
			binary.LittleEndian.PutUint16(tiff[10:], 0x0112)
			binary.LittleEndian.PutUint16(tiff[12:], 3)
			binary.LittleEndian.PutUint32(tiff[14:], 1)
			binary.LittleEndian.PutUint16(tiff[18:], test.orientation)
			segment := append([]byte("Exif\x00\x00"), tiff...)
			length := make([]byte, 2)
			binary.BigEndian.PutUint16(length, uint16(len(segment)+2))
			data := append([]byte{0xff, 0xd8, 0xff, 0xe1}, length...)
			data = append(data, segment...)
			data = append(data, jpegData.Bytes()[2:]...)

			_, width, height, err := normalizeEvidenceImage(data)
			if err != nil || width != test.wantWidth || height != test.wantHeight {
				t.Fatalf("size = %.0fx%.0f, error = %v", width, height, err)
			}
		})
	}
}
