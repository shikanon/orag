package qdrantstore

import (
	"context"
	"fmt"

	qdrant "github.com/qdrant/go-client/qdrant"
)

const expectedCollectionDistance = qdrant.Distance_Cosine

func (c *Client) CollectionExists(ctx context.Context, name string) (bool, error) {
	exists, err := c.Collections.CollectionExists(ctx, &qdrant.CollectionExistsRequest{CollectionName: name})
	if err != nil {
		return false, err
	}
	return exists.GetResult().GetExists(), nil
}

func (c *Client) EnsureCollection(ctx context.Context, name string, dimensions int) error {
	exists, err := c.CollectionExists(ctx, name)
	if err != nil {
		return err
	}
	if exists {
		return c.validateExistingCollection(ctx, name, dimensions)
	}
	_, err = c.Collections.Create(ctx, &qdrant.CreateCollection{
		CollectionName: name,
		VectorsConfig: &qdrant.VectorsConfig{Config: &qdrant.VectorsConfig_Params{
			Params: &qdrant.VectorParams{
				Size:     uint64(dimensions),
				Distance: expectedCollectionDistance,
			},
		}},
	})
	return err
}

func (c *Client) ValidateCollection(ctx context.Context, name string, dimensions int) error {
	exists, err := c.CollectionExists(ctx, name)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("qdrant collection %q is missing", name)
	}
	return c.validateExistingCollection(ctx, name, dimensions)
}

func (c *Client) validateExistingCollection(ctx context.Context, name string, dimensions int) error {
	info, err := c.Collections.Get(ctx, &qdrant.GetCollectionInfoRequest{CollectionName: name})
	if err != nil {
		return fmt.Errorf("qdrant collection %q vector config validation failed: %w", name, err)
	}
	return validateCollectionInfo(name, dimensions, info.GetResult())
}

func validateCollectionInfo(name string, dimensions int, info *qdrant.CollectionInfo) error {
	config := info.GetConfig()
	params := config.GetParams()
	vectorsConfig := params.GetVectorsConfig()
	if vectorsConfig == nil {
		return collectionConfigError(name, dimensions, "metadata is missing vectors_config")
	}

	switch vectors := vectorsConfig.GetConfig().(type) {
	case *qdrant.VectorsConfig_Params:
		vectorParams := vectors.Params
		if vectorParams == nil {
			return collectionConfigError(name, dimensions, "metadata is missing unnamed vector params")
		}
		expectedSize := uint64(dimensions)
		if vectorParams.GetSize() != expectedSize {
			return collectionConfigError(name, dimensions, fmt.Sprintf("size=%d", vectorParams.GetSize()))
		}
		if vectorParams.GetDistance() != expectedCollectionDistance {
			return collectionConfigError(name, dimensions, fmt.Sprintf("distance=%s", vectorParams.GetDistance()))
		}
		return nil
	case *qdrant.VectorsConfig_ParamsMap:
		return collectionConfigError(name, dimensions, "uses named vectors")
	default:
		return collectionConfigError(name, dimensions, "metadata is missing vector params")
	}
}

func collectionConfigError(name string, dimensions int, detail string) error {
	return fmt.Errorf(
		"qdrant collection %q vector config mismatch: %s; expected single unnamed vector size=%d distance=%s; align ARK_EMBEDDING_DIMENSIONS/embedding provider with existing data, or back up and recreate/migrate the affected Qdrant collection/volume",
		name,
		detail,
		dimensions,
		expectedCollectionDistance,
	)
}
