package recognize

import (
	"backend/cloudapi/output/github.com/yandex-cloud/go-genproto/yandex/cloud/ai/stt/v3"
	"context"
	"crypto/tls"
	"fmt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"io"
	"log"
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

type ResultRecognizer struct {
	StartTime int
	EndTime   int
	Text      string
}

func NewResult(startTime, endTime int64, text string) *ResultRecognizer {
	return &ResultRecognizer{
		StartTime: int(startTime),
		EndTime:   int(endTime),
		Text:      text,
	}
}

func initOptions() *stt.StreamingOptions {
	return &stt.StreamingOptions{
		RecognitionModel: &stt.RecognitionModelOptions{
			AudioFormat: &stt.AudioFormatOptions{
				AudioFormat: &stt.AudioFormatOptions_ContainerAudio{
					ContainerAudio: &stt.ContainerAudio{
						ContainerAudioType: stt.ContainerAudio_OGG_OPUS,
					},
				},
			},
			TextNormalization: &stt.TextNormalizationOptions{
				TextNormalization: stt.TextNormalizationOptions_TEXT_NORMALIZATION_ENABLED,
				ProfanityFilter:   true,
				LiteratureText:    false,
			},
			LanguageRestriction: &stt.LanguageRestrictionOptions{
				RestrictionType: stt.LanguageRestrictionOptions_WHITELIST,
				LanguageCode:    []string{"ru-RU"},
			},
			AudioProcessingType: stt.RecognitionModelOptions_REAL_TIME,
		},
	}
}

func SpeechKitRecognize(channelIn chan []byte, channelOut chan *ResultRecognizer) {
	defer func() {
		if err := recover(); err != nil {
			fmt.Println("[ERR] grpc", err)
			close(channelIn)
		}
	}()
	grpcConn, err := grpc.NewClient(
		"stt.api.cloud.yandex.net:443",
		grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{InsecureSkipVerify: true})),
		grpc.WithPerRPCCredentials(&tokenAuth{"Api-Key "}),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(4*1024*1025)),
	)
	defer grpcConn.Close()
	if err != nil {
		fmt.Println("Ошибка подключения", err)
	}

	client := stt.NewRecognizerClient(grpcConn)
	ctx := context.Background()
	stream, err := client.RecognizeStreaming(ctx)
	if err != nil {
		fmt.Println("Ошибка подключение к стриму распознавания", err)
	}
	err = stream.Send(&stt.StreamingRequest{
		Event: &stt.StreamingRequest_SessionOptions{
			SessionOptions: initOptions(),
		},
	})
	if err != nil {
		fmt.Println("Ошибка отправки опций", err)
	}
	fmt.Println("Начало распознавания")
	go func() {
		for audioData := range channelIn {
			err := stream.Send(&stt.StreamingRequest{
				Event: &stt.StreamingRequest_Chunk{
					Chunk: &stt.AudioChunk{Data: audioData},
				},
			})
			if err != nil {
				fmt.Println("Ошибка отправки аудио в gRPC:", err)
				break
			}
		}
		defer stream.CloseSend()
	}()

	for {
		resp, err := stream.Recv()
		if err == io.EOF {
			fmt.Println("Конец распознавания", err)
			break
		}
		if err != nil {
			log.Println("Ошибка получения ответа: ", err)
			break
		}

		if resp.GetPartial() != nil {
			if resp.GetPartial().Alternatives != nil {
				channelOut <- NewResult(
					resp.GetPartial().Alternatives[0].GetStartTimeMs(),
					resp.GetPartial().Alternatives[0].GetEndTimeMs(),
					resp.GetPartial().Alternatives[0].Text,
				)
			}
		}
	}
}
