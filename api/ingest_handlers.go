package api

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	"github.com/luma/flightrecorder/services"
)

func makeHandleEvents(ingest services.Ingest) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		var req services.EventsRequest
		if err := decodeJSONBody(c, &req); err != nil {
			writeServiceError(c, fmt.Errorf("%w: %v", services.ErrBadRequest, err))
			return
		}

		resp, err := ingest.IngestEvents(ctx, services.ProjectIDFromContext(ctx), req)
		if err != nil {
			writeServiceError(c, err)
			return
		}
		c.JSON(consts.StatusOK, resp)
	}
}

func makeHandleBugReports(ingest services.Ingest) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		var req services.BugReportRequest
		if err := decodeJSONBody(c, &req); err != nil {
			writeServiceError(c, fmt.Errorf("%w: %v", services.ErrBadRequest, err))
			return
		}

		resp, err := ingest.SubmitBugReport(ctx, services.ProjectIDFromContext(ctx), req)
		if err != nil {
			writeServiceError(c, err)
			return
		}
		c.JSON(consts.StatusOK, resp)
	}
}

func decodeJSONBody(c *app.RequestContext, out any) error {
	body := c.Request.Body()
	if strings.EqualFold(string(c.GetHeader("Content-Encoding")), "gzip") {
		reader, err := gzip.NewReader(bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("decode gzip body: %w", err)
		}
		defer func() { _ = reader.Close() }()

		body, err = io.ReadAll(reader)
		if err != nil {
			return fmt.Errorf("read gzip body: %w", err)
		}
	}

	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		return fmt.Errorf("decode JSON body: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("body must contain one JSON object")
	}
	return nil
}

func writeServiceError(c *app.RequestContext, err error) {
	status := consts.StatusInternalServerError
	message := "internal server error"

	switch {
	case errors.Is(err, services.ErrBadRequest):
		status = consts.StatusBadRequest
		message = err.Error()
	case errors.Is(err, services.ErrForbidden):
		status = consts.StatusForbidden
		message = err.Error()
	case errors.Is(err, services.ErrPayloadTooLarge):
		status = consts.StatusRequestEntityTooLarge
		message = err.Error()
	case errors.Is(err, services.ErrRateLimited):
		status = consts.StatusTooManyRequests
		message = err.Error()
	}

	c.JSON(status, map[string]string{"error": message})
}
