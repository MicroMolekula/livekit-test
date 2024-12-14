package translate

import (
	"backend/cloudapi/output/github.com/yandex-cloud/go-genproto/yandex/cloud/ai/translate/v2"
	"context"
	"crypto/tls"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

type tokenAuth struct {
	Token string
}

func (t *tokenAuth) GetRequestMetadata(ctx context.Context, uri ...string) (map[string]string, error) {
	return map[string]string{
		"authorization": t.Token,
	}, nil
}

func (t *tokenAuth) RequireTransportSecurity() bool {
	return false
}

type TranslateServ struct {
	Conn *grpc.ClientConn
}

func NewServ() (*TranslateServ, error) {
	grpcConn, err := grpc.NewClient(
		"translate.api.cloud.yandex.net:443",
		grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{InsecureSkipVerify: true})),
		grpc.WithPerRPCCredentials(&tokenAuth{"Api-Key "}),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(1024*1024)),
	)
	if err != nil {
		return nil, err
	}
	return &TranslateServ{
		Conn: grpcConn,
	}, nil
}

func (t *TranslateServ) TranslateText(ctx context.Context, text string) (string, error) {
	client := translate.NewTranslationServiceClient(t.Conn)
	response, err := client.Translate(ctx, &translate.TranslateRequest{
		SourceLanguageCode: "ru",
		TargetLanguageCode: "en",
		Texts:              []string{text},
	})
	if err != nil {
		return "", err
	}
	return response.Translations[0].Text, nil
}

func (t *TranslateServ) CloseConn() error {
	err := t.Conn.Close()
	if err != nil {
		return err
	}
	return nil
}
