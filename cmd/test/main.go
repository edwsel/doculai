package main

import (
	"bytes"
	"fmt"
	"image/png"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/klippa-app/go-pdfium"
	"github.com/klippa-app/go-pdfium/enums"
	"github.com/klippa-app/go-pdfium/references"
	"github.com/klippa-app/go-pdfium/requests"
	"github.com/klippa-app/go-pdfium/webassembly"
	"golang.org/x/text/encoding/charmap"
)

// PDFType - тип PDF-документа
type PDFType int

const (
	TypeUnknown PDFType = iota
	TypeText            // Обычный текстовый PDF
	TypeScan            // Скан (только изображения)
	TypeHybrid          // Гибрид (текст + изображения)
)

func (t PDFType) String() string {
	switch t {
	case TypeText:
		return "ТЕКСТОВЫЙ"
	case TypeScan:
		return "СКАН"
	case TypeHybrid:
		return "ГИБРИД (текст + изображения)"
	default:
		return "НЕИЗВЕСТНО"
	}
}

// PageAnalysis - результат анализа одной страницы
type PageAnalysis struct {
	PageNumber   int
	HasText      bool
	HasImage     bool
	TextObjects  int
	ImageObjects int
	TextLength   int
	Type         PDFType
}

// minTextChars - минимум непустых символов, чтобы считать страницу текстовой.
const minTextChars = 20

// ExtractedImage - извлечённое изображение
type ExtractedImage struct {
	PageNumber int
	Index      int
	Data       []byte
	Format     string // расширение: jpg, png, ...
}

var (
	pool     pdfium.Pool
	instance pdfium.Pdfium
)

func init() {
	var err error
	pool, err = webassembly.Init(webassembly.Config{
		MinIdle:  1,
		MaxIdle:  1,
		MaxTotal: 1,
	})
	if err != nil {
		log.Fatalf("Ошибка инициализации PDFium WebAssembly: %v", err)
	}

	instance, err = pool.GetInstance(time.Second * 60)
	if err != nil {
		log.Fatalf("Ошибка получения instance PDFium: %v", err)
	}
}

// AnalyzePDF - анализирует PDF: для каждой страницы определяет наличие
// текстовых и графических объектов, классифицирует страницы.
func AnalyzePDF(filePath string) ([]PageAnalysis, PDFType, error) {
	pdfBytes, err := os.ReadFile(filePath)
	if err != nil {
		return nil, TypeUnknown, fmt.Errorf("чтение файла: %w", err)
	}

	doc, err := instance.OpenDocument(&requests.OpenDocument{File: &pdfBytes})
	if err != nil {
		return nil, TypeUnknown, fmt.Errorf("открытие PDF: %w", err)
	}
	defer instance.FPDF_CloseDocument(&requests.FPDF_CloseDocument{Document: doc.Document})

	pageCountResp, err := instance.FPDF_GetPageCount(&requests.FPDF_GetPageCount{Document: doc.Document})
	if err != nil {
		return nil, TypeUnknown, fmt.Errorf("получение кол-ва страниц: %w", err)
	}

	pageResults := make([]PageAnalysis, 0, pageCountResp.PageCount)
	totalTextPages, totalImagePages, totalHybridPages := 0, 0, 0

	for i := 0; i < pageCountResp.PageCount; i++ {
		analysis := analyzePage(i, doc.Document)

		switch analysis.Type {
		case TypeHybrid:
			totalHybridPages++
		case TypeText:
			totalTextPages++
		case TypeScan:
			totalImagePages++
		}

		pageResults = append(pageResults, analysis)
	}

	var overallType PDFType
	switch {
	case totalHybridPages > 0:
		overallType = TypeHybrid
	case totalTextPages >= totalImagePages && totalTextPages > 0:
		overallType = TypeText
	case totalImagePages > 0:
		overallType = TypeScan
	default:
		overallType = TypeUnknown
	}

	return pageResults, overallType, nil
}

// analyzePage - анализирует одну страницу (0-indexed).
func analyzePage(index int, doc references.FPDF_DOCUMENT) PageAnalysis {
	analysis := PageAnalysis{PageNumber: index + 1}

	loadedPage, err := instance.FPDF_LoadPage(&requests.FPDF_LoadPage{Document: doc, Index: index})
	if err != nil {
		log.Printf("Страница %d: ошибка загрузки: %v", index+1, err)
		return analysis
	}
	defer instance.FPDF_ClosePage(&requests.FPDF_ClosePage{Page: loadedPage.Page})

	page := requests.Page{ByReference: &loadedPage.Page}

	countResp, err := instance.FPDFPage_CountObjects(&requests.FPDFPage_CountObjects{Page: page})
	if err != nil {
		log.Printf("Страница %d: ошибка подсчёта объектов: %v", index+1, err)
		return analysis
	}

	for objIdx := 0; objIdx < countResp.Count; objIdx++ {
		objResp, err := instance.FPDFPage_GetObject(&requests.FPDFPage_GetObject{Page: page, Index: objIdx})
		if err != nil {
			continue
		}

		typeResp, err := instance.FPDFPageObj_GetType(&requests.FPDFPageObj_GetType{PageObject: objResp.PageObject})
		if err != nil {
			continue
		}

		switch typeResp.Type {
		case enums.FPDF_PAGEOBJ_TEXT:
			analysis.TextObjects++
		case enums.FPDF_PAGEOBJ_IMAGE:
			analysis.ImageObjects++
		}
	}

	textResp, err := instance.GetPageText(&requests.GetPageText{Page: page})
	if err == nil {
		analysis.TextLength = len(strings.TrimSpace(textResp.Text))
	}

	analysis.HasText = analysis.TextLength > minTextChars
	analysis.HasImage = analysis.ImageObjects > 0

	switch {
	case analysis.HasText && analysis.HasImage:
		analysis.Type = TypeHybrid
	case analysis.HasText:
		analysis.Type = TypeText
	default:
		// Нет извлекаемого текста — считаем страницей-сканом
		// (включая векторные/недоступные объекты), рендерим целиком.
		analysis.Type = TypeScan
	}

	return analysis
}

// loadPageRef - загружает страницу и возвращает Page + функцию закрытия.
func loadPageRef(doc references.FPDF_DOCUMENT, index int) (requests.Page, func(), error) {
	loadedPage, err := instance.FPDF_LoadPage(&requests.FPDF_LoadPage{Document: doc, Index: index})
	if err != nil {
		return requests.Page{}, nil, err
	}
	page := requests.Page{ByReference: &loadedPage.Page}
	closeFn := func() { instance.FPDF_ClosePage(&requests.FPDF_ClosePage{Page: loadedPage.Page}) }
	return page, closeFn, nil
}

// ExtractText - извлекает текст из всех страниц
func ExtractText(filePath string) ([]string, error) {
	pdfBytes, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	doc, err := instance.OpenDocument(&requests.OpenDocument{File: &pdfBytes})
	if err != nil {
		return nil, err
	}
	defer instance.FPDF_CloseDocument(&requests.FPDF_CloseDocument{Document: doc.Document})

	pageCountResp, err := instance.FPDF_GetPageCount(&requests.FPDF_GetPageCount{Document: doc.Document})
	if err != nil {
		return nil, err
	}

	pages := make([]string, 0, pageCountResp.PageCount)
	for i := 0; i < pageCountResp.PageCount; i++ {
		textResp, err := instance.GetPageText(&requests.GetPageText{
			Page: requests.Page{ByIndex: &requests.PageByIndex{Document: doc.Document, Index: i}},
		})
		if err != nil {
			pages = append(pages, "")
			continue
		}
		pages = append(pages, fixCyrillicMojibake(textResp.Text))
	}

	return pages, nil
}

// ExtractImages - извлекает встроенные изображения из всех страниц
func ExtractImages(filePath string) ([]ExtractedImage, error) {
	pdfBytes, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	doc, err := instance.OpenDocument(&requests.OpenDocument{File: &pdfBytes})
	if err != nil {
		return nil, err
	}
	defer instance.FPDF_CloseDocument(&requests.FPDF_CloseDocument{Document: doc.Document})

	pageCountResp, err := instance.FPDF_GetPageCount(&requests.FPDF_GetPageCount{Document: doc.Document})
	if err != nil {
		return nil, err
	}

	var images []ExtractedImage

	for i := 0; i < pageCountResp.PageCount; i++ {
		page, closePage, err := loadPageRef(doc.Document, i)
		if err != nil {
			log.Printf("Страница %d: %v", i+1, err)
			continue
		}

		countResp, err := instance.FPDFPage_CountObjects(&requests.FPDFPage_CountObjects{Page: page})
		if err != nil {
			closePage()
			continue
		}

		imgIdx := 0
		for objIdx := 0; objIdx < countResp.Count; objIdx++ {
			objResp, err := instance.FPDFPage_GetObject(&requests.FPDFPage_GetObject{Page: page, Index: objIdx})
			if err != nil {
				continue
			}

			typeResp, err := instance.FPDFPageObj_GetType(&requests.FPDFPageObj_GetType{PageObject: objResp.PageObject})
			if err != nil || typeResp.Type != enums.FPDF_PAGEOBJ_IMAGE {
				continue
			}

			dataResp, err := instance.FPDFImageObj_GetImageDataDecoded(&requests.FPDFImageObj_GetImageDataDecoded{
				ImageObject: objResp.PageObject,
			})
			if err != nil || len(dataResp.Data) == 0 {
				continue
			}

			images = append(images, ExtractedImage{
				PageNumber: i + 1,
				Index:      imgIdx,
				Data:       dataResp.Data,
				Format:     detectImageFormat(dataResp.Data),
			})
			imgIdx++
		}
		closePage()
	}

	return images, nil
}

// RenderScanPages - рендерит указанные страницы как изображения (для сканов)
func RenderScanPages(filePath string, pageIndices []int, outDir string, dpi int) ([]string, error) {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return nil, err
	}

	pdfBytes, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	doc, err := instance.OpenDocument(&requests.OpenDocument{File: &pdfBytes})
	if err != nil {
		return nil, err
	}
	defer instance.FPDF_CloseDocument(&requests.FPDF_CloseDocument{Document: doc.Document})

	var paths []string

	for _, pageIdx := range pageIndices {
		renderResp, err := instance.RenderPageInDPI(&requests.RenderPageInDPI{
			DPI: dpi,
			Page: requests.Page{
				ByIndex: &requests.PageByIndex{Document: doc.Document, Index: pageIdx},
			},
		})
		if err != nil {
			log.Printf("Рендер страницы %d: %v", pageIdx+1, err)
			continue
		}

		outPath := filepath.Join(outDir, fmt.Sprintf("page_%d.png", pageIdx+1))
		f, err := os.Create(outPath)
		if err != nil {
			renderResp.Cleanup()
			return paths, err
		}

		if err := png.Encode(f, renderResp.Result.Image); err != nil {
			f.Close()
			renderResp.Cleanup()
			return paths, err
		}
		f.Close()
		renderResp.Cleanup()

		paths = append(paths, outPath)
	}

	return paths, nil
}

// detectImageFormat - определяет формат по magic bytes
func detectImageFormat(data []byte) string {
	switch {
	case len(data) >= 3 && bytes.Equal(data[0:3], []byte{0xFF, 0xD8, 0xFF}):
		return "jpg"
	case len(data) >= 8 && bytes.Equal(data[0:8], []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}):
		return "png"
	case len(data) >= 2 && data[0] == 0x42 && data[1] == 0x4D:
		return "bmp"
	case len(data) >= 4 && bytes.Equal(data[0:4], []byte("GIF8")):
		return "gif"
	case len(data) >= 4 && bytes.Equal(data[0:4], []byte("II*")) || bytes.Equal(data[0:4], []byte("MM\x00*")):
		return "tiff"
	default:
		return "bin"
	}
}

// cp1251Table содержит отображение байтов CP1251 (0x80–0xFF) в Unicode.
// Заполняется в init из charmap.Windows1251.
var cp1251Table [128]rune

func init() {
	dec := charmap.Windows1251.NewDecoder()
	for b := 0x80; b <= 0xFF; b++ {
		out, err := dec.Bytes([]byte{byte(b)})
		if err == nil {
			if rs := []rune(string(out)); len(rs) == 1 {
				cp1251Table[b-0x80] = rs[0]
			}
		}
	}
}

// fixCyrillicMojibake восстанавливает кириллицу из PDF со сломанным
// ToUnicode CMap. PDFium возвращает байты CP1251 (Windows-1251),
// интерпретированные как Latin-1 supplemental (U+00C0–U+00FF), и текст
// выглядит как «ÄÀÍÍÀß» вместо «ДАННАЯ». Преобразование посимвольное,
// поэтому работает для смешанного содержимого, где часть объектов уже
// отдаёт корректную кириллицу (U+0400–U+04FF, остаётся нетронутой).
func fixCyrillicMojibake(s string) string {
	var sb strings.Builder
	sb.Grow(len(s))
	changed := false
	for _, r := range s {
		if r >= 0xC0 && r <= 0xFF {
			sb.WriteRune(cp1251Table[int(r)-0x80])
			changed = true
		} else {
			sb.WriteRune(r)
		}
	}
	if !changed {
		return s
	}
	return sb.String()
}

func main() {
	filePath := "./storage/uhvat.pdf"
	if len(os.Args) > 1 {
		filePath = os.Args[1]
	}

	outDir := "./storage/out"
	baseName := strings.TrimSuffix(filepath.Base(filePath), filepath.Ext(filePath))

	fmt.Printf("Анализ PDF: %s\n\n", filePath)

	// 1. Анализ
	pageResults, overallType, err := AnalyzePDF(filePath)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("=== Анализ по страницам ===")
	for _, page := range pageResults {
		fmt.Printf("Страница %d: тип=%s, текст=%v(объектов=%d, символов=%d), изображения=%v(объектов=%d)\n",
			page.PageNumber, page.Type, page.HasText, page.TextObjects, page.TextLength, page.HasImage, page.ImageObjects)
	}
	fmt.Printf("\nОбщий тип документа: %s\n\n", overallType)
	// 2. Извлечение текста
	textPages, err := ExtractText(filePath)
	if err != nil {
		log.Printf("Извлечение текста: %v", err)
	} else {
		textPath := filepath.Join(outDir, baseName+".txt")
		var fullText strings.Builder
		for i, t := range textPages {
			fullText.WriteString(fmt.Sprintf("=== Страница %d ===\n", i+1))
			fullText.WriteString(t)
			fullText.WriteString("\n\n")
		}
		if err := os.WriteFile(textPath, []byte(fullText.String()), 0o644); err != nil {
			log.Printf("Запись текста: %v", err)
		} else {
			fmt.Printf("Текст сохранён: %s (%d страниц)\n", textPath, len(textPages))
		}
	}

	// 3. Извлечение встроенных изображений
	images, err := ExtractImages(filePath)
	if err != nil {
		log.Printf("Извлечение изображений: %v", err)
	} else if len(images) > 0 {
		imgDir := filepath.Join(outDir, baseName+"_images")
		os.MkdirAll(imgDir, 0o755)
		for _, img := range images {
			imgPath := filepath.Join(imgDir, fmt.Sprintf("page%d_%d.%s", img.PageNumber, img.Index, img.Format))
			if err := os.WriteFile(imgPath, img.Data, 0o644); err != nil {
				log.Printf("Запись изображения %s: %v", imgPath, err)
			}
		}
		fmt.Printf("Извлечено изображений: %d -> %s\n", len(images), imgDir)
	} else {
		fmt.Println("Встроенные изображения не найдены")
	}

	// 4. Рендер сканированных страниц
	var scanPages []int
	for _, p := range pageResults {
		if p.Type == TypeScan {
			scanPages = append(scanPages, p.PageNumber-1)
		}
	}
	if len(scanPages) > 0 {
		scanDir := filepath.Join(outDir, baseName+"_scans")
		paths, err := RenderScanPages(filePath, scanPages, scanDir, 200)
		if err != nil {
			log.Printf("Рендер сканов: %v", err)
		} else {
			fmt.Printf("Отрендерено страниц-сканов: %d -> %s\n", len(paths), scanDir)
			for _, p := range paths {
				fmt.Printf("  %s\n", p)
			}
		}
	} else {
		fmt.Println("Сканированных страниц не обнаружено")
	}
}
