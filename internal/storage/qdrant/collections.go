package qdrantstore

import (
	"context"

	qdrant "github.com/qdrant/go-client/qdrant"
)

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
		return nil
	}
	_, err = c.Collections.Create(ctx, &qdrant.CreateCollection{
		CollectionName: name,
		VectorsConfig: &qdrant.VectorsConfig{Config: &qdrant.VectorsConfig_Params{
			Params: &qdrant.VectorParams{
				Size:     uint64(dimensions),
				Distance: qdrant.Distance_Cosine,
			},
		}},
	})
	return err
}
