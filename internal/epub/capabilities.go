package epub

import (
	"bytes"
	"os/exec"
	"strings"
)

type ConversionCapabilities struct {
	Python            bool     `json:"python"`
	OCRmyPDF          bool     `json:"ocrmypdf"`
	Mutool            bool     `json:"mutool"`
	PDFToText         bool     `json:"pdftotext"`
	PDFToPPM          bool     `json:"pdftoppm"`
	EbookConvert      bool     `json:"ebookConvert"`
	PyMuPDF4LLM       bool     `json:"pymupdf4llm"`
	Docling           bool     `json:"docling"`
	Marker            bool     `json:"marker"`
	Nougat            bool     `json:"nougat"`
	StructuredEngines []string `json:"structuredEngines"`
	PageRenderers     []string `json:"pageRenderers"`
}

func DetectConversionCapabilities() ConversionCapabilities {
	caps := ConversionCapabilities{
		OCRmyPDF:     binaryAvailable("ocrmypdf"),
		Mutool:       binaryAvailable("mutool"),
		PDFToText:    binaryAvailable("pdftotext"),
		PDFToPPM:     binaryAvailable("pdftoppm"),
		EbookConvert: binaryAvailable("ebook-convert"),
	}

	pythonBin, pythonOK := detectPythonBinary()
	caps.Python = pythonOK == nil
	if caps.Python {
		caps.PyMuPDF4LLM = pythonModuleAvailable(pythonBin, "pymupdf4llm")
		caps.Docling = pythonModuleAvailable(pythonBin, "docling.document_converter")
		caps.Marker = binaryAvailable("marker_single") || pythonModuleAvailable(pythonBin, "marker.converters.pdf")
		caps.Nougat = binaryAvailable("nougat") || pythonModuleAvailable(pythonBin, "nougat")
	}

	if caps.PyMuPDF4LLM {
		caps.StructuredEngines = append(caps.StructuredEngines, "pymupdf4llm")
	}
	if caps.Docling {
		caps.StructuredEngines = append(caps.StructuredEngines, "docling")
	}
	if caps.Marker {
		caps.StructuredEngines = append(caps.StructuredEngines, "marker")
	}
	if caps.Nougat {
		caps.StructuredEngines = append(caps.StructuredEngines, "nougat")
	}
	if caps.Mutool {
		caps.PageRenderers = append(caps.PageRenderers, "mutool")
	}
	if caps.PDFToPPM {
		caps.PageRenderers = append(caps.PageRenderers, "pdftoppm")
	}
	return caps
}

func (c ConversionCapabilities) Summary() string {
	parts := []string{}
	if c.EbookConvert {
		parts = append(parts, "ebook-convert")
	}
	if c.OCRmyPDF {
		parts = append(parts, "ocrmypdf")
	}
	if c.Mutool {
		parts = append(parts, "mutool")
	}
	if c.PDFToText {
		parts = append(parts, "pdftotext")
	}
	if len(c.StructuredEngines) > 0 {
		parts = append(parts, "structured="+strings.Join(c.StructuredEngines, ","))
	}
	if len(c.PageRenderers) > 0 {
		parts = append(parts, "renderers="+strings.Join(c.PageRenderers, ","))
	}
	if len(parts) == 0 {
		return "no optional converters detected; pure-Go fallback only"
	}
	return strings.Join(parts, " | ")
}

func binaryAvailable(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func pythonModuleAvailable(pythonBin, module string) bool {
	cmd := exec.Command(pythonBin, "-c", "import "+module)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	return cmd.Run() == nil
}
