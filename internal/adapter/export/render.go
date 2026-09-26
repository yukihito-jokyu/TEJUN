package export

import (
	"bytes"
	_ "embed"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"strings"

	"github.com/signintech/gopdf"
)

//go:embed assets/NotoSansJP.ttf
var notoSansJP []byte

type Document struct {
	Title         string   `json:"title"`
	Overview      string   `json:"overview"`
	Prerequisites []string `json:"prerequisites"`
	Steps         []Step   `json:"steps"`
}

type Step struct {
	Title        string        `json:"title"`
	Description  string        `json:"description"`
	Command      string        `json:"command"`
	Notes        []string      `json:"notes"`
	EvidenceRefs []EvidenceRef `json:"evidenceRefs"`
}

type EvidenceRef struct {
	EvidenceID  string `json:"evidenceId"`
	DisplayName string `json:"displayName"`
	Included    bool   `json:"included"`
}

func Render(format string, documentJSON []byte, output io.Writer) error {
	return RenderWithEvidence(format, documentJSON, "", nil, output)
}

func RenderWithEvidence(
	format string,
	documentJSON []byte,
	procedureID string,
	read func(string, string) ([]byte, error),
	output io.Writer,
) error {
	var document Document
	if err := json.Unmarshal(documentJSON, &document); err != nil {
		return err
	}

	if strings.TrimSpace(document.Title) == "" {
		return fmt.Errorf("手順書のtitleがありません")
	}

	switch format {
	case "markdown":
		_, err := io.WriteString(output, markdown(document))
		return err
	case "pdf":
		return pdf(document, procedureID, read, output)
	default:
		return fmt.Errorf("未対応の出力形式です")
	}
}

func markdown(document Document) string {
	var b strings.Builder
	b.WriteString("# " + plain(document.Title) + "\n\n")

	if document.Overview != "" {
		b.WriteString("## 概要\n\n" + plain(document.Overview) + "\n\n")
	}

	if len(document.Prerequisites) > 0 {
		b.WriteString("## 前提条件\n\n")

		for _, prerequisite := range document.Prerequisites {
			b.WriteString("- " + plain(prerequisite) + "\n")
		}

		b.WriteByte('\n')
	}

	for index, step := range document.Steps {
		fmt.Fprintf(&b, "## %d. %s\n\n", index+1, plain(step.Title))

		if step.Description != "" {
			b.WriteString(plain(step.Description) + "\n\n")
		}

		if step.Command != "" {
			b.WriteString("```text\n" + strings.ReplaceAll(step.Command, "```", "` ` `") + "\n```\n\n")
		}

		for _, note := range step.Notes {
			b.WriteString("> 注意: " + plain(note) + "\n")
		}

		if len(step.Notes) > 0 {
			b.WriteByte('\n')
		}

		for _, evidence := range step.EvidenceRefs {
			b.WriteString("- 証跡: " + plain(evidence.DisplayName) + "\n")
		}

		if len(step.EvidenceRefs) > 0 {
			b.WriteByte('\n')
		}
	}

	return b.String()
}

func plain(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	value = strings.ReplaceAll(value, "<", "&lt;")
	value = strings.ReplaceAll(value, ">", "&gt;")

	return strings.ReplaceAll(value, "\n", " ")
}

func pdf(document Document, procedureID string, read func(string, string) ([]byte, error), output io.Writer) error {
	p := &gopdf.GoPdf{}
	p.Start(gopdf.Config{PageSize: *gopdf.PageSizeA4})

	if err := p.AddTTFFontByReader("NotoSansJP", bytes.NewReader(notoSansJP)); err != nil {
		return err
	}

	contentsPages := (len(document.Steps) + 32) / 33
	if contentsPages < 1 {
		contentsPages = 1
	}

	if err := p.SetFont("NotoSansJP", "", 12); err != nil {
		return err
	}

	for range contentsPages {
		p.AddPage()

		if err := p.Cell(&gopdf.Rect{W: 1, H: 1}, " "); err != nil {
			return err
		}
	}

	p.AddPage()

	y := 40.0
	newPage := func() { p.AddPage(); y = 40 }

	writeLine := func(value string, kind string) error {
		for _, paragraph := range strings.Split(value, "\n") {
			if paragraph == "" {
				paragraph = " "
			}

			lines, err := p.SplitText(paragraph, 510)
			if err != nil {
				return err
			}

			if len(lines) == 0 {
				lines = []string{""}
			}

			for _, line := range lines {
				if y+20 > 790 {
					newPage()
				}

				if kind == "code" {
					p.SetFillColor(241, 243, 245)
					p.RectFromLowerLeft(38, 842-y-20, 520, 20)
				}

				if kind == "note" {
					p.SetTextColor(160, 55, 20)
				}

				p.SetXY(42, y)

				if err := p.Cell(&gopdf.Rect{W: 510, H: 20}, line); err != nil {
					return err
				}

				p.SetTextColor(0, 0, 0)

				y += 20
			}
		}

		y += 5

		return nil
	}
	if err := writeLine(document.Title, "heading"); err != nil {
		return err
	}

	if err := writeLine(document.Overview, "body"); err != nil {
		return err
	}

	for _, prerequisite := range document.Prerequisites {
		if err := writeLine("前提条件: "+prerequisite, "body"); err != nil {
			return err
		}
	}

	stepPages := make([]int, 0, len(document.Steps))
	for index, step := range document.Steps {
		if y+20 > 790 {
			newPage()
		}

		stepPages = append(stepPages, p.GetNumberOfPages())

		for _, value := range []struct{ text, kind string }{
			{fmt.Sprintf("%d. %s", index+1, step.Title), "heading"},
			{step.Description, "body"},
			{step.Command, "code"},
		} {
			if value.text != "" {
				if err := writeLine(value.text, value.kind); err != nil {
					return err
				}
			}
		}

		for _, note := range step.Notes {
			if err := writeLine("注意: "+note, "note"); err != nil {
				return err
			}
		}

		for _, evidence := range step.EvidenceRefs {
			if err := writeLine("証跡: "+evidence.DisplayName, "body"); err != nil {
				return err
			}

			if evidence.Included {
				if evidence.EvidenceID == "" || read == nil {
					return fmt.Errorf("証跡画像を読み取れません")
				}

				data, err := read(procedureID, evidence.EvidenceID)
				if err != nil {
					return err
				}

				imageBytes, width, height, err := normalizeEvidenceImage(data)
				if err != nil {
					return err
				}

				if y+height > 790 {
					newPage()
				}

				holder, err := gopdf.ImageHolderByBytes(imageBytes)
				if err != nil {
					return err
				}

				if err := p.ImageByHolder(holder, 42, y, &gopdf.Rect{W: width, H: height}); err != nil {
					return err
				}

				y += height + 12
			}
		}
	}

	for page := 1; page <= contentsPages; page++ {
		if err := p.SetPage(page); err != nil {
			return err
		}

		y = 40

		if err := writeLine("目次", "heading"); err != nil {
			return err
		}

		for index := (page - 1) * 33; index < page*33 && index < len(document.Steps); index++ {
			title := []rune(document.Steps[index].Title)
			if len(title) > 32 {
				title = append(title[:31], '…')
			}

			if err := writeLine(
				fmt.Sprintf("%d. %s  %d", index+1, string(title), stepPages[index]),
				"body",
			); err != nil {
				return err
			}
		}
	}

	for page := 1; page <= p.GetNumberOfPages(); page++ {
		if err := p.SetPage(page); err != nil {
			return err
		}

		p.SetXY(40, 808)

		if err := p.Cell(&gopdf.Rect{W: 510, H: 20}, fmt.Sprintf("%d / %d", page, p.GetNumberOfPages())); err != nil {
			return err
		}
	}

	_, err := p.WriteTo(output)

	return err
}

func normalizeEvidenceImage(data []byte) ([]byte, float64, float64, error) {
	img, format, err := image.Decode(bytes.NewReader(data))
	if err != nil || (format != "png" && format != "jpeg") {
		return nil, 0, 0, fmt.Errorf("証跡画像が不正です")
	}

	width, height := img.Bounds().Dx(), img.Bounds().Dy()
	if width <= 0 || height <= 0 || int64(width)*int64(height) > 100_000_000 {
		return nil, 0, 0, fmt.Errorf("証跡画像の寸法が不正です")
	}

	if format == "jpeg" {
		img = orientImage(img, exifOrientation(data))
		width, height = img.Bounds().Dx(), img.Bounds().Dy()
	}
	// PDF内の画像を最大幅・高さへ縮小し、元画像のbyteを直接埋め込まない。
	scale := min(510/float64(width), 600/float64(height), 1)
	dstWidth, dstHeight := max(1, int(float64(width)*scale)), max(1, int(float64(height)*scale))

	dst := image.NewRGBA(image.Rect(0, 0, dstWidth, dstHeight))
	for y := range dstHeight {
		for x := range dstWidth {
			dst.Set(x, y, img.At(img.Bounds().Min.X+x*width/dstWidth, img.Bounds().Min.Y+y*height/dstHeight))
		}
	}

	var output bytes.Buffer
	if format == "jpeg" {
		err = jpeg.Encode(&output, dst, &jpeg.Options{Quality: 85})
	} else {
		err = png.Encode(&output, dst)
	}

	return output.Bytes(), float64(dstWidth), float64(dstHeight), err
}

func exifOrientation(data []byte) int {
	for offset := 2; offset+4 < len(data) && data[offset] == 0xff; {
		marker := data[offset+1]
		if marker == 0xda || marker == 0xd9 {
			break
		}

		length := int(binary.BigEndian.Uint16(data[offset+2:]))
		if length < 2 || offset+2+length > len(data) {
			break
		}

		segment := data[offset+4 : offset+2+length]
		if marker == 0xe1 && len(segment) >= 14 && bytes.Equal(segment[:6], []byte("Exif\x00\x00")) {
			tiff := segment[6:]

			var order binary.ByteOrder

			switch string(tiff[:2]) {
			case "II":
				order = binary.LittleEndian
			case "MM":
				order = binary.BigEndian
			default:
				return 1
			}

			if order.Uint16(tiff[2:]) != 42 {
				return 1
			}

			ifd := int(order.Uint32(tiff[4:]))
			if ifd < 8 || ifd+2 > len(tiff) {
				return 1
			}

			count := int(order.Uint16(tiff[ifd:]))
			for index := range count {
				entry := ifd + 2 + index*12
				if entry+12 > len(tiff) {
					return 1
				}

				if order.Uint16(tiff[entry:]) == 0x0112 && order.Uint16(tiff[entry+2:]) == 3 &&
					order.Uint32(tiff[entry+4:]) == 1 {
					value := int(order.Uint16(tiff[entry+8:]))
					if value >= 1 && value <= 8 {
						return value
					}
				}
			}
		}

		offset += 2 + length
	}

	return 1
}

func orientImage(src image.Image, orientation int) image.Image {
	if orientation <= 1 || orientation > 8 {
		return src
	}

	w, h := src.Bounds().Dx(), src.Bounds().Dy()
	if orientation >= 5 {
		w, h = h, w
	}

	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	sw, sh := src.Bounds().Dx(), src.Bounds().Dy()

	for y := range h {
		for x := range w {
			sx, sy := x, y

			switch orientation {
			case 2:
				sx = sw - 1 - x
			case 3:
				sx, sy = sw-1-x, sh-1-y
			case 4:
				sy = sh - 1 - y
			case 5:
				sx, sy = y, x
			case 6:
				sx, sy = y, sh-1-x
			case 7:
				sx, sy = sw-1-y, sh-1-x
			case 8:
				sx, sy = sw-1-y, x
			}

			dst.Set(x, y, src.At(src.Bounds().Min.X+sx, src.Bounds().Min.Y+sy))
		}
	}

	return dst
}
