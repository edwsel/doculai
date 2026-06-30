package pdf

import (
	"fmt"
	"sync"
	"time"

	"github.com/klippa-app/go-pdfium"
	"github.com/klippa-app/go-pdfium/references"
	"github.com/klippa-app/go-pdfium/requests"
	"github.com/klippa-app/go-pdfium/webassembly"
)

const (
	// poolMaxTotal is the maximum number of PDFium instances kept in the pool.
	poolMaxTotal = 1
	// getInstanceTimeout is how long getInstance waits for a pool instance.
	getInstanceTimeout = 60 * time.Second
)

var (
	pool     pdfium.Pool
	poolOnce sync.Once
	poolErr  error
)

// initPool lazily initializes the shared PDFium WebAssembly pool exactly once.
func initPool() {
	poolOnce.Do(func() {
		pool, poolErr = webassembly.Init(webassembly.Config{
			MinIdle:  1,
			MaxIdle:  1,
			MaxTotal: poolMaxTotal,
		})
	})
}

// getInstance returns a PDFium instance borrowed from the shared pool along
// with a release function that returns it. The caller MUST defer the release
// function when done.
func getInstance() (pdfium.Pdfium, func(), error) {
	initPool()
	if poolErr != nil {
		return nil, nil, fmt.Errorf("initializing pdfium: %w", poolErr)
	}

	instance, err := pool.GetInstance(getInstanceTimeout)
	if err != nil {
		return nil, nil, fmt.Errorf("getting pdfium instance: %w", err)
	}

	released := false
	release := func() {
		if released {
			return
		}
		released = true
		_ = instance.Close()
	}
	return instance, release, nil
}

// openDocument opens a PDF document from raw bytes and returns the document
// reference, the borrowed PDFium instance, and a combined cleanup function
// that closes the document and releases the instance. The caller MUST defer
// the cleanup function.
func openDocument(data []byte) (references.FPDF_DOCUMENT, pdfium.Pdfium, func(), error) {
	instance, release, err := getInstance()
	if err != nil {
		return references.FPDF_DOCUMENT(""), nil, nil, err
	}

	doc, err := instance.OpenDocument(&requests.OpenDocument{File: &data})
	if err != nil {
		release()
		return references.FPDF_DOCUMENT(""), nil, nil, fmt.Errorf("opening pdf document: %w", err)
	}

	cleaned := false
	cleanup := func() {
		if cleaned {
			return
		}
		cleaned = true
		_, _ = instance.FPDF_CloseDocument(&requests.FPDF_CloseDocument{Document: doc.Document})
		release()
	}
	return doc.Document, instance, cleanup, nil
}
