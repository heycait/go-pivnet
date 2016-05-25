package errors

import (
	"github.com/pivotal-cf-experimental/go-pivnet"
	"github.com/pivotal-cf-experimental/go-pivnet/cmd/pivnet/printer"
)

//go:generate counterfeiter . ErrorHandler

type ErrorHandler interface {
	HandleError(err error) error
}

type errorHandler struct {
	format  string
	printer printer.Printer
}

func NewErrorHandler(format string, printer printer.Printer) ErrorHandler {
	return &errorHandler{
		format:  format,
		printer: printer,
	}
}

func (h errorHandler) HandleError(err error) error {
	if err == nil {
		return nil
	}

	var message string

	switch err.(type) {
	case pivnet.ErrUnauthorized:
		message = "Please log in first"
	case pivnet.ErrNotFound:
		message = "Not found"
	default:
		message = err.Error()
	}

	switch h.format {
	case printer.PrintAsJSON:
		e := h.printer.PrintJSON(message)
		if e != nil {
			return e
		}
		return err
	case printer.PrintAsYAML:
		e := h.printer.PrintYAML(message)
		if e != nil {
			return e
		}
		return err
	default:
		h.printer.Println(message)
		return err
	}

	return nil
}