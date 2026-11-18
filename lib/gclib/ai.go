package gclib

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	aiplatform "cloud.google.com/go/aiplatform/apiv1"
	"cloud.google.com/go/aiplatform/apiv1/aiplatformpb"
	"golang.org/x/oauth2"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/structpb"
)

// GcpAiConfig configures Vertex AI Prediction & Endpoint clients via cloud.google.com/go/aiplatform/apiv1.
type GcpAiConfig struct {
	ProjectID       string
	Location        string
	EndpointURL     string
	Token           string
	TokenSource     oauth2.TokenSource
	CredentialsJSON []byte
	HTTPClient      *http.Client
	GRPCConn        *grpc.ClientConn
	ClientOptions   []option.ClientOption
}

// EndpointSummary represents a Vertex AI endpoint resource summary.
type EndpointSummary struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
}

// GcpAi wraps official Vertex AI PredictionClient and EndpointClient.
type GcpAi struct {
	Config           GcpAiConfig
	PredictionClient *aiplatform.PredictionClient
	EndpointClient   *aiplatform.EndpointClient
}

// NewGcpAi initializes Vertex AI PredictionClient and EndpointClient backed by cloud.google.com/go/aiplatform/apiv1.
func NewGcpAi(cfg GcpAiConfig) (*GcpAi, error) {
	if strings.TrimSpace(cfg.ProjectID) == "" {
		return nil, errors.New("gclib: ProjectID is required")
	}
	if strings.TrimSpace(cfg.Location) == "" && strings.TrimSpace(cfg.EndpointURL) == "" && cfg.GRPCConn == nil {
		return nil, errors.New("gclib: Location, EndpointURL, or GRPCConn is required")
	}

	ctx := context.Background()
	opts, err := buildGoogleClientOptions(
		ctx,
		cfg.TokenSource,
		cfg.Token,
		cfg.CredentialsJSON,
		nil,
		cfg.GRPCConn,
		cfg.ClientOptions,
		aiplatform.DefaultAuthScopes()...,
	)
	if err != nil {
		return nil, err
	}

	if cfg.GRPCConn == nil {
		ep := strings.TrimSpace(cfg.EndpointURL)
		if ep == "" {
			ep = fmt.Sprintf("%s-aiplatform.googleapis.com:443", cfg.Location)
		}
		opts = append([]option.ClientOption{option.WithEndpoint(ep)}, opts...)
	}

	predClient, err := aiplatform.NewPredictionClient(ctx, opts...)
	if err != nil {
		return nil, err
	}
	epClient, err := aiplatform.NewEndpointClient(ctx, opts...)
	if err != nil {
		_ = predClient.Close()
		return nil, err
	}

	return &GcpAi{
		Config:           cfg,
		PredictionClient: predClient,
		EndpointClient:   epClient,
	}, nil
}

// Predict invokes a Vertex AI endpoint via PredictionClient.Predict and returns decoded predictions.
func (a *GcpAi) Predict(ctx context.Context, endpointID string, instances []map[string]interface{}) ([]interface{}, error) {
	if strings.TrimSpace(endpointID) == "" {
		return nil, errors.New("gclib: endpointID cannot be empty")
	}
	resourcePath := strings.TrimLeft(endpointID, "/")
	if !strings.HasPrefix(resourcePath, "projects/") {
		resourcePath = fmt.Sprintf("projects/%s/locations/%s/endpoints/%s", a.Config.ProjectID, a.Config.Location, resourcePath)
	}

	pbInstances := make([]*structpb.Value, 0, len(instances))
	for _, inst := range instances {
		val, err := structpb.NewValue(inst)
		if err != nil {
			return nil, err
		}
		pbInstances = append(pbInstances, val)
	}

	resp, err := a.PredictionClient.Predict(ctx, &aiplatformpb.PredictRequest{
		Endpoint:  resourcePath,
		Instances: pbInstances,
	})
	if err != nil {
		return nil, err
	}

	preds := make([]interface{}, len(resp.GetPredictions()))
	for i, p := range resp.GetPredictions() {
		preds[i] = p.AsInterface()
	}
	return preds, nil
}

// ListEndpoints lists Vertex AI endpoints in Config.ProjectID and Config.Location via EndpointClient.ListEndpoints.
func (a *GcpAi) ListEndpoints(ctx context.Context) ([]EndpointSummary, error) {
	parent := fmt.Sprintf("projects/%s/locations/%s", a.Config.ProjectID, a.Config.Location)
	it := a.EndpointClient.ListEndpoints(ctx, &aiplatformpb.ListEndpointsRequest{
		Parent: parent,
	})

	var out []EndpointSummary
	for {
		ep, err := it.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, err
		}
		out = append(out, EndpointSummary{
			Name:        ep.GetName(),
			DisplayName: ep.GetDisplayName(),
		})
	}
	if out == nil {
		out = []EndpointSummary{}
	}
	return out, nil
}

// Close closes the underlying Vertex AI gRPC clients.
func (a *GcpAi) Close() error {
	var firstErr error
	if a.PredictionClient != nil {
		if err := a.PredictionClient.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if a.EndpointClient != nil {
		if err := a.EndpointClient.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
