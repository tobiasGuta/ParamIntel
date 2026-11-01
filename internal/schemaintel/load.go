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
	return ParseWithLimit(data, DefaultMaxDocumentBytes)
}

// ParseWithLimit preserves the existing parser contract after a hard
// pre-model byte admission. It does not claim to bound YAML expansion.
func ParseWithLimit(data []byte, limit int64) (*Document, error) {
	if err := validateDocumentBytes(data, limit); err != nil {
		return nil, err
	}
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
