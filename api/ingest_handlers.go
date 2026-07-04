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
		if err := decodeIngestBody(c, &req); err != nil {
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
		if err := decodeIngestBody(c, &req); err != nil {
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

// decodeJSONBody decodes a single JSON object with strict unknown-field
// rejection. Used by the admin and MCP APIs, whose clients are few and
// updatable, so an unknown field is a useful early typo signal.
func decodeJSONBody(c *app.RequestContext, out any) error {
	return decodeBody(c, out, true)
}

// decodeIngestBody decodes a single JSON object leniently (unknown top-level
// fields are ignored). Used by the ingestion API: game clients are many and
// unpatchable, and a strict unknown-field 400 rejects the whole batch — a poison
// of the same class as the incident this addresses. Per-event validation plus
// the rejections response provide better, non-blocking feedback instead.
func decodeIngestBody(c *app.RequestContext, out any) error {
	return decodeBody(c, out, false)
}

func decodeBody(c *app.RequestContext, out any, disallowUnknownFields bool) error {
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
	if disallowUnknownFields {
		decoder.DisallowUnknownFields()
	}
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
