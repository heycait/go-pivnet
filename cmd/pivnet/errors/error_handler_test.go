package errors_test

import (
	"bytes"
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pivotal-cf-experimental/go-pivnet"
	"github.com/pivotal-cf-experimental/go-pivnet/cmd/pivnet/errors"
	"github.com/pivotal-cf-experimental/go-pivnet/cmd/pivnet/printer"
)

var _ = Describe("ErrorHandler", func() {
	var (
		errorHandler errors.ErrorHandler

		format    string
		outWriter *bytes.Buffer
		logWriter *bytes.Buffer

		inputErr error
	)

	BeforeEach(func() {
		outWriter = &bytes.Buffer{}
		logWriter = &bytes.Buffer{}

		format = printer.PrintAsTable

		inputErr = fmt.Errorf("some error")
	})

	JustBeforeEach(func() {
		errorHandler = errors.NewErrorHandler(
			format,
			outWriter,
			logWriter,
		)
	})

	It("returns ErrAlreadyHandled", func() {
		err := errorHandler.HandleError(inputErr)

		Expect(err).To(Equal(errors.ErrAlreadyHandled))
	})

	It("writes to outWriter", func() {
		_ = errorHandler.HandleError(inputErr)

		Expect(outWriter.String()).To(Equal(fmt.Sprintln("some error")))
	})

	Context("when the error is nil", func() {
		BeforeEach(func() {
			inputErr = nil
		})

		It("returns nil", func() {
			err := errorHandler.HandleError(inputErr)

			Expect(err).NotTo(HaveOccurred())
		})

		It("does not write to printer", func() {
			_ = errorHandler.HandleError(nil)

			Expect(outWriter.String()).To(BeEmpty())
			Expect(logWriter.String()).To(BeEmpty())
		})
	})

	Describe("print as JSON", func() {
		BeforeEach(func() {
			format = printer.PrintAsJSON
		})

		It("writes to logWriter", func() {
			_ = errorHandler.HandleError(inputErr)

			Expect(logWriter.String()).To(Equal(fmt.Sprintln("some error")))
		})
	})

	Describe("print as YAML", func() {
		BeforeEach(func() {
			format = printer.PrintAsYAML
		})

		It("writes to logWriter", func() {
			_ = errorHandler.HandleError(inputErr)

			Expect(logWriter.String()).To(Equal(fmt.Sprintln("some error")))
		})
	})

	Describe("Handling specific Pivnet errors", func() {
		Describe("pivnet.ErrUnauthorized", func() {
			BeforeEach(func() {
				inputErr = pivnet.ErrUnauthorized{}
			})

			It("retuns custom message", func() {
				_ = errorHandler.HandleError(inputErr)

				Expect(outWriter.String()).To(Equal(fmt.Sprintln("Failed to authenticate - please provide valid API token")))
			})
		})

		Describe("pivnet.ErrNotFound", func() {
			BeforeEach(func() {
				inputErr = pivnet.ErrNotFound{
					ResponseCode: 404,
					Message:      "something not found",
				}
			})

			It("retuns custom message", func() {
				_ = errorHandler.HandleError(inputErr)

				Expect(outWriter.String()).To(Equal(fmt.Sprintln("Pivnet error: something not found")))
			})
		})
	})
})
