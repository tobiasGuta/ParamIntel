package schemaintel

import (
	"fmt"
	"io"
	"log/slog"

	libopenapi "github.com/pb33f/libopenapi"
	"github.com/pb33f/libopenapi/datamodel"
	v3high "github.com/pb33f/libopenapi/datamodel/high/v3"
)

type Document struct {
	model *libopenapi.DocumentModel[v3high.Document]
}

func Parse(data []byte) (*Document, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("openapi document is empty")
	}

	cfg := datamodel.NewDocumentConfiguration()
	cfg.AllowFileReferences = false
	cfg.AllowRemoteReferences = false
	cfg.BasePath = ""
	cfg.BaseURL = nil
	cfg.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))

	doc, err := libopenapi.NewDocumentWithConfiguration(data, cfg)
	if err != nil {
		return nil, fmt.Errorf("parse openapi document: %w", err)
	}
	// libopenapi v0.38.7+ changed BuildV3Model to return a single error instead of []error.
	model, buildErr := doc.BuildV3Model()
	if buildErr != nil {
		return nil, fmt.Errorf("build openapi model: %w", buildErr)
	}
	if model == nil {
		return nil, fmt.Errorf("build openapi model: no OpenAPI 3 model returned")
	}
	return &Document{model: model}, nil
}

func (d *Document) Version() string {
	if d == nil || d.model == nil {
		return ""
	}
	return d.model.Model.Version
}
