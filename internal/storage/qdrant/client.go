package qdrantstore

import (
	"context"
	"fmt"

	qdrant "github.com/qdrant/go-client/qdrant"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

type Config struct {
	Host   string
	Port   int
	APIKey string
	UseTLS bool
}

type PointsClient interface {
	Upsert(ctx context.Context, in *qdrant.UpsertPoints, opts ...grpc.CallOption) (*qdrant.PointsOperationResponse, error)
	Search(ctx context.Context, in *qdrant.SearchPoints, opts ...grpc.CallOption) (*qdrant.SearchResponse, error)
}

type Client struct {
	Conn        *grpc.ClientConn
	Qdrant      qdrant.QdrantClient
	Collections qdrant.CollectionsClient
	Points      PointsClient
}

func Open(ctx context.Context, cfg Config) (*Client, error) {
	target := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	opts := []grpc.DialOption{grpc.WithBlock()}
	if cfg.UseTLS {
		opts = append(opts, grpc.WithTransportCredentials(credentials.NewTLS(nil)))
	} else {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}
	if cfg.APIKey != "" {
		opts = append(opts, grpc.WithPerRPCCredentials(apiKeyCredentials{key: cfg.APIKey, secure: cfg.UseTLS}))
	}
	conn, err := grpc.DialContext(ctx, target, opts...)
	if err != nil {
		return nil, err
	}
	return &Client{
		Conn:        conn,
		Qdrant:      qdrant.NewQdrantClient(conn),
		Collections: qdrant.NewCollectionsClient(conn),
		Points:      qdrant.NewPointsClient(conn),
	}, nil
}

type apiKeyCredentials struct {
	key    string
	secure bool
}

func (c apiKeyCredentials) GetRequestMetadata(context.Context, ...string) (map[string]string, error) {
	return map[string]string{"api-key": c.key}, nil
}

func (c apiKeyCredentials) RequireTransportSecurity() bool {
	return c.secure
}
