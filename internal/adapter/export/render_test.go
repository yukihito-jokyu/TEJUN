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
		{name: "html", format: "html", prefix: "<!doctype html>"},
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

func TestHTMLEscapesContentAndEmbedsEvidence(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))

	var encoded bytes.Buffer
	if err := png.Encode(&encoded, img); err != nil {
		t.Fatal(err)
	}

	document := []byte(
		`{"title":"<script>alert(1)</script>","overview":"<&>","prerequisites":["<準備>"],"steps":[{"title":"開始","description":"<&>","command":"echo '<>&'","notes":["<注意>"],"evidenceRefs":[{"evidenceId":"image","displayName":"<画像>","included":true},{"evidenceId":"text","displayName":"記録","included":true}]}]}`,
	)

	tests := []struct {
		name      string
		kind      string
		wantError bool
	}{
		{name: "valid"},
		{name: "corrupt image", kind: "corrupt", wantError: true},
		{name: "missing reader", kind: "missing", wantError: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var (
				output bytes.Buffer
				read   func(string, string) (EvidenceContent, error)
			)
			if test.kind != "missing" {
				read = func(_, id string) (EvidenceContent, error) {
					if id == "text" {
						return EvidenceContent{Kind: "text", Data: []byte("<observed>")}, nil
					}

					if test.kind == "corrupt" {
						return EvidenceContent{Kind: "image", Data: []byte("broken")}, nil
					}

					return EvidenceContent{Kind: "image", Data: encoded.Bytes()}, nil
				}
			}

			err := RenderWithEvidence("html", document, "procedure", read, &output)
			if (err != nil) != test.wantError {
				t.Fatalf("error = %v, wantError %t", err, test.wantError)
			}

			if test.wantError {
				return
			}

			for _, want := range []string{"&lt;script&gt;", "&lt;&amp;&gt;", "&lt;準備&gt;", "&lt;注意&gt;", "&lt;observed&gt;", "data:image/png;base64,"} {
				if !strings.Contains(output.String(), want) {
					t.Errorf("HTML lacks %q", want)
				}
			}

			if strings.Contains(output.String(), "<script>") {
				t.Fatal("unescaped script")
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
		read    func(string, string) (EvidenceContent, error)
		wantErr bool
	}{
		{"available", func(procedure, evidence string) (EvidenceContent, error) {
			if procedure != "procedure" || evidence != "evidence" {
				t.Fatal("wrong relation")
			}

			return EvidenceContent{Kind: "image", Data: encoded.Bytes()}, nil
		}, false},
		{"corrupt image", func(string, string) (EvidenceContent, error) {
			return EvidenceContent{Kind: "image", Data: []byte("broken")}, nil
		}, true},
		{"included text", func(string, string) (EvidenceContent, error) {
			return EvidenceContent{Kind: "text", Data: []byte("observed")}, nil
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

			if !test.wantErr && test.name == "available" && !bytes.Contains(output.Bytes(), []byte("/Subtype /Image")) {
				t.Fatal("PDF does not contain an image")
			}
		})
	}
}

func TestPDFEvidenceTextLimit(t *testing.T) {
	document := []byte(
		`{"title":"手順","steps":[{"title":"確認","evidenceRefs":[{"evidenceId":"evidence","included":true}]}]}`,
	)

	for _, test := range []struct {
		name      string
		size      int
		wantError bool
	}{
		{name: "at limit", size: 64 << 10},
		{name: "over limit", size: 64<<10 + 1, wantError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer

			err := RenderWithEvidence("pdf", document, "procedure", func(string, string) (EvidenceContent, error) {
				return EvidenceContent{Kind: "text", Data: bytes.Repeat([]byte("a"), test.size)}, nil
			}, &output)
			if (err != nil) != test.wantError {
				t.Fatalf("error = %v", err)
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
