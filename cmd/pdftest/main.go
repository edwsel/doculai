package main

import (
	"fmt"
	"os"
	"time"

	"github.com/klippa-app/go-pdfium/requests"
	"github.com/klippa-app/go-pdfium/webassembly"
)

func main() {
	pool, err := webassembly.Init(webassembly.Config{MinIdle: 1, MaxIdle: 1, MaxTotal: 1})
	if err != nil {
		panic(err)
	}
	inst, err := pool.GetInstance(time.Second * 60)
	if err != nil {
		panic(err)
	}

	b, _ := os.ReadFile("./storage/uhvat.pdf")
	doc, err := inst.OpenDocument(&requests.OpenDocument{File: &b})
	if err != nil {
		panic(err)
	}
	defer inst.FPDF_CloseDocument(&requests.FPDF_CloseDocument{Document: doc.Document})

	pc, _ := inst.FPDF_GetPageCount(&requests.FPDF_GetPageCount{Document: doc.Document})
	fmt.Println("pages:", pc.PageCount)

	// count WITHOUT render
	lp, _ := inst.FPDF_LoadPage(&requests.FPDF_LoadPage{Document: doc.Document, Index: 0})
	pg := lp.Page
	c1, err := inst.FPDFPage_CountObjects(&requests.FPDFPage_CountObjects{Page: requests.Page{ByReference: &pg}})
	fmt.Printf("count-no-render: %d err=%v\n", c1.Count, err)

	// text
	t, err := inst.GetPageText(&requests.GetPageText{Page: requests.Page{ByReference: &pg}})
	fmt.Printf("text len=%d err=%v\n", len(t.Text), err)
	inst.FPDF_ClosePage(&requests.FPDF_ClosePage{Page: lp.Page})

	// fresh page WITH render first
	lp2, _ := inst.FPDF_LoadPage(&requests.FPDF_LoadPage{Document: doc.Document, Index: 0})
	pg2 := lp2.Page
	r, _ := inst.RenderPageInDPI(&requests.RenderPageInDPI{DPI: 72, Page: requests.Page{ByReference: &pg2}})
	r.Cleanup()
	c2, err := inst.FPDFPage_CountObjects(&requests.FPDFPage_CountObjects{Page: requests.Page{ByReference: &pg2}})
	fmt.Printf("count-after-render: %d err=%v\n", c2.Count, err)
}
