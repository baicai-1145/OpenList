package modelscope

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/OpenListTeam/OpenList/internal/driver"
	"github.com/OpenListTeam/OpenList/internal/errs"
	"github.com/OpenListTeam/OpenList/internal/model"
	"github.com/OpenListTeam/OpenList/pkg/utils"
	"github.com/go-resty/resty/v2"
)

const (
	API_ENDPOINT = "https://www.modelscope.cn"
)

type ModelScope struct {
	model.Storage
	Addition
	client *resty.Client
}

func (d *ModelScope) Config() driver.Config {
	return config
}

func (d *ModelScope) GetAddition() driver.Additional {
	return &d.Addition
}

func (d *ModelScope) Init(ctx context.Context) error {
	d.client = resty.New()
	return nil
}

func (d *ModelScope) Drop(ctx context.Context) error {
	return nil
}

func (d *ModelScope) List(ctx context.Context, dir model.Obj, args model.ListArgs) ([]model.Obj, error) {
	path := dir.GetPath()
	if path == d.GetRootPath() {
		path = ""
	}

	apiURL := fmt.Sprintf("%s/api/v1/models/%s/repo/files?Revision=%s&Recursive=false&Root=%s",
		API_ENDPOINT, d.ModelID, d.Revision, path)
	utils.Log.Infof("ModelScope List API URL: %s", apiURL)
	resp, err := d.client.R().SetContext(ctx).Get(apiURL)
	if err != nil {
		utils.Log.Errorf("modelscope list api request error: %+v", err)
		return nil, err
	}

	if resp.StatusCode() != http.StatusOK {
		utils.Log.Errorf("modelscope list api response status error: %d, body: %s", resp.StatusCode(), string(resp.Body()))
		return nil, fmt.Errorf("failed to list files: status code %d", resp.StatusCode())
	}

	var fileListResp FileListResponse
	if err := json.Unmarshal(resp.Body(), &fileListResp); err != nil {
		utils.Log.Errorf("modelscope list api unmarshal error: %+v", err)
		utils.Log.Errorf("modelscope list api response body: %s", string(resp.Body()))
		return nil, err
	}

	if !fileListResp.Success {
		utils.Log.Errorf("modelscope list api logic error: %s (RequestId: %s)", fileListResp.Message, fileListResp.RequestId)
		return nil, fmt.Errorf("modelscope api error: %s", fileListResp.Message)
	}

	return filesToObjs(fileListResp.Data.Files), nil
}

func (d *ModelScope) Link(ctx context.Context, file model.Obj, args model.LinkArgs) (*model.Link, error) {
	apiURL := fmt.Sprintf("%s/api/v1/models/%s/repo?Revision=%s&FilePath=%s",
		API_ENDPOINT, d.ModelID, d.Revision, file.GetPath())
	utils.Log.Infof("ModelScope Link API URL: %s, Redirect: %v", apiURL, args.Redirect)

	if !args.Redirect {
		// Proxy mode
		return &model.Link{
			URL: apiURL,
		}, nil
	}

	// Redirect mode
	client := resty.New().SetRedirectPolicy(resty.NoRedirectPolicy())
	resp, err := client.R().SetContext(ctx).Get(apiURL)

	if err != nil && resp.StatusCode() != http.StatusFound && resp.StatusCode() != http.StatusOK {
		utils.Log.Errorf("modelscope link api request error: %+v, status: %d", err, resp.StatusCode())
		return nil, err
	}

	var finalURL string
	if resp.StatusCode() == http.StatusFound {
		finalURL = resp.Header().Get("Location")
		if finalURL == "" {
			utils.Log.Errorf("modelscope link api error: Location header not found in 302 redirect response")
			return nil, fmt.Errorf("failed to get download link: Location header not found")
		}
	} else {
		finalURL = apiURL
	}

	link := &model.Link{
		URL: finalURL,
	}
	return link, nil
}

func (d *ModelScope) MakeDir(ctx context.Context, parentDir model.Obj, dirName string) (model.Obj, error) {
	return nil, errs.NotImplement
}

func (d *ModelScope) Move(ctx context.Context, srcObj, dstDir model.Obj) (model.Obj, error) {
	return nil, errs.NotImplement
}

func (d *ModelScope) Rename(ctx context.Context, srcObj model.Obj, newName string) (model.Obj, error) {
	return nil, errs.NotImplement
}

func (d *ModelScope) Copy(ctx context.Context, srcObj, dstDir model.Obj) (model.Obj, error) {
	return nil, errs.NotImplement
}

func (d *ModelScope) Remove(ctx context.Context, obj model.Obj) error {
	return errs.NotImplement
}

func (d *ModelScope) Put(ctx context.Context, dstDir model.Obj, file model.FileStreamer, up driver.UpdateProgress) (model.Obj, error) {
	return nil, errs.NotImplement
}

func (d *ModelScope) GetArchiveMeta(ctx context.Context, obj model.Obj, args model.ArchiveArgs) (model.ArchiveMeta, error) {
	// TODO get archive file meta-info, return errs.NotImplement to use an internal archive tool, optional
	return nil, errs.NotImplement
}

func (d *ModelScope) ListArchive(ctx context.Context, obj model.Obj, args model.ArchiveInnerArgs) ([]model.Obj, error) {
	// TODO list args.InnerPath in the archive obj, return errs.NotImplement to use an internal archive tool, optional
	return nil, errs.NotImplement
}

func (d *ModelScope) Extract(ctx context.Context, obj model.Obj, args model.ArchiveInnerArgs) (*model.Link, error) {
	// TODO return link of file args.InnerPath in the archive obj, return errs.NotImplement to use an internal archive tool, optional
	return nil, errs.NotImplement
}

func (d *ModelScope) ArchiveDecompress(ctx context.Context, srcObj, dstDir model.Obj, args model.ArchiveDecompressArgs) ([]model.Obj, error) {
	// TODO extract args.InnerPath path in the archive srcObj to the dstDir location, optional
	// a folder with the same name as the archive file needs to be created to store the extracted results if args.PutIntoNewDir
	// return errs.NotImplement to use an internal archive tool
	return nil, errs.NotImplement
}

func (d *ModelScope) GetRootPath() string {
	return d.ModelID
}

//func (d *ModelScope) Other(ctx context.Context, args model.OtherArgs) (interface{}, error) {
//	return nil, errs.NotSupport
//}

var _ driver.Driver = (*ModelScope)(nil)
