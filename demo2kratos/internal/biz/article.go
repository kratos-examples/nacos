package biz

import (
	"context"
	"math/rand/v2"

	"github.com/brianvoe/gofakeit/v7"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/yylego/kratos-ebz/ebzkratos"
	demo1student "github.com/yylego/kratos-examples/demo1kratos/api/student"
	pb "github.com/yylego/kratos-examples/demo2kratos/api/article"
	"github.com/yylego/kratos-examples/demo2kratos/internal/data"
)

type Article struct {
	ID        int64
	Title     string
	Content   string
	StudentID int64
}

type ArticleUsecase struct {
	data            *data.Data
	demo1GrpcClient *data.Demo1GrpcClient
	demo1HttpClient *data.Demo1HttpClient
	log             *log.Helper
}

func NewArticleUsecase(data *data.Data, demo1GrpcClient *data.Demo1GrpcClient, demo1HttpClient *data.Demo1HttpClient, logger log.Logger) *ArticleUsecase {
	return &ArticleUsecase{data: data, demo1GrpcClient: demo1GrpcClient, demo1HttpClient: demo1HttpClient, log: log.NewHelper(logger)}
}

func (uc *ArticleUsecase) CreateArticle(ctx context.Context, a *Article) (*Article, *ebzkratos.Ebz) {
	var res Article
	if err := gofakeit.Struct(&res); err != nil {
		return nil, ebzkratos.New(pb.ErrorArticleCreateFailure("fake: %v", err))
	}
	// 这里是两个demo，grpc和http，随便用哪个都行，使用随机演示
	if rand.IntN(2) == 0 {
		// cp from https://github.com/go-kratos/examples/blob/61daed1ec4d5a94d689bc8fab9bc960c6af73ead/registry/nacos/client/main.go#L52
		resp, err := uc.demo1GrpcClient.GetStudentClient().CreateStudent(ctx, &demo1student.CreateStudentRequest{
			Name: res.Title,
		})
		if err != nil {
			return nil, ebzkratos.New(pb.ErrorServerError("grpc: %v", err))
		}
		res.Title = "message:[grpc-resp:" + resp.GetStudent().GetName() + "]"
	} else {
		// cp from https://github.com/go-kratos/kratos/blob/d6f5f00cf562b46322b0ed42d183b1b873c0a68f/transport/http/client_test.go#L354
		resp, err := uc.demo1HttpClient.GetStudentClient().CreateStudent(ctx, &demo1student.CreateStudentRequest{
			Name: res.Title,
		})
		if err != nil {
			return nil, ebzkratos.New(pb.ErrorServerError("http: %v", err))
		}
		res.Title = "message:[http-resp:" + resp.GetStudent().GetName() + "]"
	}
	return &res, nil
}

func (uc *ArticleUsecase) UpdateArticle(ctx context.Context, a *Article) (*Article, *ebzkratos.Ebz) {
	var res Article
	if err := gofakeit.Struct(&res); err != nil {
		return nil, ebzkratos.New(pb.ErrorServerError("fake: %v", err))
	}
	return &res, nil
}

func (uc *ArticleUsecase) DeleteArticle(ctx context.Context, id int64) *ebzkratos.Ebz {
	return nil
}

func (uc *ArticleUsecase) GetArticle(ctx context.Context, id int64) (*Article, *ebzkratos.Ebz) {
	var res Article
	if err := gofakeit.Struct(&res); err != nil {
		return nil, ebzkratos.New(pb.ErrorServerError("fake: %v", err))
	}
	return &res, nil
}

func (uc *ArticleUsecase) ListArticles(ctx context.Context, page int32, pageSize int32) ([]*Article, int32, *ebzkratos.Ebz) {
	var items []*Article
	gofakeit.Slice(&items)
	return items, int32(len(items)), nil
}
