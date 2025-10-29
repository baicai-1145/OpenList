package modelscope

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	neturl "net/url"
	stdpath "path"
	"strings"
	"time"

	"github.com/OpenListTeam/OpenList/v4/internal/driver"
	"github.com/OpenListTeam/OpenList/v4/internal/errs"
	"github.com/OpenListTeam/OpenList/v4/internal/model"
	"github.com/OpenListTeam/OpenList/v4/pkg/utils"
	"github.com/go-resty/resty/v2"
)

const (
	apiEndpoint         = "https://www.modelscope.cn"
	resourceTypeModel   = "model"
	resourceTypeDataset = "dataset"
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
	d.ModelID = strings.TrimSpace(d.ModelID)
	if d.ModelID == "" {
		return fmt.Errorf("model_id is required")
	}
	d.ResourceType = strings.ToLower(strings.TrimSpace(d.ResourceType))
	if d.ResourceType == "" {
		d.ResourceType = resourceTypeModel
	}
	if d.ResourceType != resourceTypeModel && d.ResourceType != resourceTypeDataset {
		return fmt.Errorf("unsupported resource_type: %s", d.ResourceType)
	}
	if strings.TrimSpace(d.Revision) == "" {
		d.Revision = "master"
	}
	d.DefaultRoot = strings.Trim(strings.TrimSpace(d.DefaultRoot), "/")
	return nil
}

func (d *ModelScope) Drop(ctx context.Context) error {
	return nil
}

func (d *ModelScope) List(ctx context.Context, dir model.Obj, args model.ListArgs) ([]model.Obj, error) {
	path := dir.GetPath()
	if path == d.GetRootPath() {
		if d.DefaultRoot != "" {
			path = d.DefaultRoot
		} else {
			path = ""
		}
	}

	// For datasets, prefer /repo/tree with Path parameter.
	if d.ResourceType == resourceTypeDataset {
		return d.listDatasetTree(ctx, path)
	}

	var lastErr error
	segments := d.resourceSegmentsCandidates()
	for _, seg := range segments {
		for idx, revision := range d.revisionCandidates() {
			// Try GET first
			getURL := fmt.Sprintf("%s/api/v1/%s/%s/repo/files?Revision=%s&Recursive=false&Root=%s", apiEndpoint, seg, d.ModelID, revision, path)
			utils.Log.Infof("ModelScope List API URL: %s (GET try %d)", getURL, idx+1)
			resp, err := d.client.R().SetContext(ctx).Get(getURL)
			if err == nil && resp.StatusCode() == http.StatusOK {
				var fileListResp FileListResponse
				uerr := json.Unmarshal(resp.Body(), &fileListResp)
				if uerr == nil && (fileListResp.Success || fileListResp.Code == 200) {
					return filesToObjs(fileListResp.Data.Files), nil
				}
				if uerr != nil {
					utils.Log.Errorf("modelscope list api unmarshal error: %+v", uerr)
					utils.Log.Errorf("modelscope list api response body: %s", string(resp.Body()))
					lastErr = uerr
				} else if !fileListResp.Success && fileListResp.Code != 200 {
					utils.Log.Errorf("modelscope list api logic error: %s (RequestId: %s, Code: %d)", fileListResp.Message, fileListResp.RequestID, fileListResp.Code)
					lastErr = fmt.Errorf("modelscope api error: %s", fileListResp.Message)
				}
			} else if err != nil {
				utils.Log.Errorf("modelscope list api request error: %+v", err)
				lastErr = err
			} else {
				utils.Log.Errorf("modelscope list api response status error: %d, body: %s", resp.StatusCode(), string(resp.Body()))
				lastErr = fmt.Errorf("failed to list files: status code %d", resp.StatusCode())
			}

			// If GET not successful or returned 405/404, try POST fallback
			if lastErr != nil {
				status := 0
				if resp != nil {
					status = resp.StatusCode()
				}
				if status == http.StatusMethodNotAllowed || status == http.StatusNotFound {
					postURL := fmt.Sprintf("%s/api/v1/%s/%s/repo/files", apiEndpoint, seg, d.ModelID)
					payload := map[string]interface{}{"Revision": revision, "Recursive": false, "Root": path}
					utils.Log.Infof("ModelScope List API URL: %s (POST try %d)", postURL, idx+1)
					resp2, err2 := d.client.R().SetHeader("Content-Type", "application/json").SetBody(payload).SetContext(ctx).Post(postURL)
					if err2 == nil && resp2.StatusCode() == http.StatusOK {
						var fileListResp2 FileListResponse
						uerr := json.Unmarshal(resp2.Body(), &fileListResp2)
						if uerr == nil && (fileListResp2.Success || fileListResp2.Code == 200) {
							return filesToObjs(fileListResp2.Data.Files), nil
						}
						if uerr != nil {
							utils.Log.Errorf("modelscope list api(unmarshal,post) error: %+v", uerr)
							utils.Log.Errorf("modelscope list api(post) response body: %s", string(resp2.Body()))
							lastErr = uerr
						} else if !fileListResp2.Success && fileListResp2.Code != 200 {
							utils.Log.Errorf("modelscope list api(post) logic error: %s (RequestId: %s, Code: %d)", fileListResp2.Message, fileListResp2.RequestID, fileListResp2.Code)
							lastErr = fmt.Errorf("modelscope api error: %s", fileListResp2.Message)
						}
					} else if err2 != nil {
						utils.Log.Errorf("modelscope list api(post) request error: %+v", err2)
						lastErr = err2
					} else {
						utils.Log.Errorf("modelscope list api(post) status error: %d, body: %s", resp2.StatusCode(), string(resp2.Body()))
						lastErr = fmt.Errorf("failed to list files(post): status code %d", resp2.StatusCode())
					}
				}
			}
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("modelscope list failed with all strategies")
	}
	return nil, lastErr
}

func (d *ModelScope) Link(ctx context.Context, file model.Obj, args model.LinkArgs) (*model.Link, error) {
	client := resty.New().SetRedirectPolicy(resty.NoRedirectPolicy())
	var lastErr error
	segments := d.resourceSegmentsCandidates()
	// For datasets, build repo-relative file path by trimming driver root
	filePath := file.GetPath()
	if d.ResourceType == resourceTypeDataset {
		filePath = d.repoRelativePath(filePath)
	}
	for _, seg := range segments {
		for idx, revision := range d.revisionCandidates() {
			encodedFilePath := neturl.QueryEscape(filePath)
			apiURL := fmt.Sprintf("%s/api/v1/%s/%s/repo?Revision=%s&FilePath=%s", apiEndpoint, seg, d.ModelID, revision, encodedFilePath)
			utils.Log.Infof("ModelScope Link API URL: %s, Redirect: %v (GET try %d)", apiURL, args.Redirect, idx+1)

			resp, err := client.R().SetContext(ctx).Get(apiURL)
			if err == nil || (resp != nil && resp.StatusCode() == http.StatusFound) {
				switch resp.StatusCode() {
				case http.StatusFound:
					finalURL := resp.Header().Get("Location")
					if finalURL == "" {
						utils.Log.Errorf("modelscope link api error: Location header not found in 302 redirect response")
						lastErr = fmt.Errorf("failed to get download link: Location header not found")
						// try POST fallback below
					} else {
						if args.Redirect {
							return &model.Link{URL: finalURL}, nil
						}
						return &model.Link{URL: apiURL}, nil
					}
				case http.StatusOK:
					return &model.Link{URL: apiURL}, nil
				}
			} else {
				utils.Log.Errorf("modelscope link api request error: %+v", err)
				lastErr = err
			}

			// POST fallback if GET not usable or 405/404
			status := 0
			if resp != nil {
				status = resp.StatusCode()
			}
			if status == http.StatusMethodNotAllowed || status == http.StatusNotFound || lastErr != nil {
				postURL := fmt.Sprintf("%s/api/v1/%s/%s/repo", apiEndpoint, seg, d.ModelID)
				payload := map[string]interface{}{"Revision": revision, "FilePath": filePath}
				utils.Log.Infof("ModelScope Link API URL: %s, Redirect: %v (POST try %d)", postURL, args.Redirect, idx+1)
				resp2, err2 := client.R().SetHeader("Content-Type", "application/json").SetBody(payload).SetContext(ctx).Post(postURL)
				if err2 == nil {
					switch resp2.StatusCode() {
					case http.StatusFound:
						finalURL := resp2.Header().Get("Location")
						if finalURL == "" {
							utils.Log.Errorf("modelscope link api(post) error: Location header not found in 302 redirect response")
							lastErr = fmt.Errorf("failed to get download link(post): Location header not found")
						} else {
							if args.Redirect {
								return &model.Link{URL: finalURL}, nil
							}
							return &model.Link{URL: postURL}, nil
						}
					case http.StatusOK:
						return &model.Link{URL: postURL}, nil
					default:
						utils.Log.Errorf("modelscope link api(post) response status error: %d", resp2.StatusCode())
						lastErr = fmt.Errorf("failed to get download link(post): status code %d", resp2.StatusCode())
					}
				} else {
					utils.Log.Errorf("modelscope link api(post) request error: %+v", err2)
					lastErr = err2
				}
			}
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("modelscope link failed with all strategies")
	}
	return nil, lastErr
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

func (d *ModelScope) Put(ctx context.Context, dstDir model.Obj, file model.FileStreamer, updater driver.UpdateProgress) (model.Obj, error) {
	return nil, errs.NotImplement
}

func (d *ModelScope) GetArchiveMeta(ctx context.Context, obj model.Obj, args model.ArchiveArgs) (model.ArchiveMeta, error) {
	return nil, errs.NotImplement
}

func (d *ModelScope) ListArchive(ctx context.Context, obj model.Obj, args model.ArchiveInnerArgs) ([]model.Obj, error) {
	return nil, errs.NotImplement
}

func (d *ModelScope) Extract(ctx context.Context, obj model.Obj, args model.ArchiveInnerArgs) (*model.Link, error) {
	return nil, errs.NotImplement
}

func (d *ModelScope) ArchiveDecompress(ctx context.Context, srcObj, dstDir model.Obj, args model.ArchiveDecompressArgs) ([]model.Obj, error) {
	return nil, errs.NotImplement
}

func (d *ModelScope) GetRootPath() string {
	return d.ModelID
}

func (d *ModelScope) resourceSegment() string {
	if d.ResourceType == resourceTypeDataset {
		return "datasets"
	}
	return "models"
}

func (d *ModelScope) resourceSegmentsCandidates() []string {
	seg := d.resourceSegment()
	if seg == "datasets" {
		return []string{"datasets", "dataset"}
	}
	if seg == "models" {
		return []string{"models", "model"}
	}
	return []string{seg}
}

func (d *ModelScope) revisionCandidates() []string {
	rev := strings.TrimSpace(d.Revision)
	if rev == "" {
		return []string{"master", "main"}
	}
	candidates := []string{rev}
	if strings.ToLower(rev) != "master" {
		candidates = append(candidates, "master")
	}
	if strings.ToLower(rev) != "main" {
		candidates = append(candidates, "main")
	}
	return candidates
}

// listDatasetTree lists files for datasets using the /repo/tree endpoint.
func (d *ModelScope) listDatasetTree(ctx context.Context, relPath string) ([]model.Obj, error) {
	// Normalize to repo-relative path (trim mount root / ModelID prefix)
	raw := strings.TrimSpace(relPath)
	root := strings.Trim(d.GetRootPath(), "/")
	path := strings.TrimPrefix(raw, "/")
	if path == root {
		path = ""
	}
	if strings.HasPrefix(path, root+"/") {
		path = strings.TrimPrefix(path, root+"/")
	}
	path = strings.TrimPrefix(path, "/")

	utils.Log.Infof("ModelScope dataset list: relPath='%s' root='%s' normalizedPath='%s'", relPath, root, path)

	segments := d.resourceSegmentsCandidates()
	encodedPath := neturl.QueryEscape(path)
	var lastErr error
	for _, seg := range segments {
		for idx, revision := range d.revisionCandidates() {
			// Try both Root and Path parameter names; Root first (observed effective on many datasets)
			for _, paramName := range []string{"Root", "Path"} {
				apiURL := fmt.Sprintf("%s/api/v1/%s/%s/repo/tree?Revision=%s&%s=%s&PageNumber=1&PageSize=1000", apiEndpoint, seg, d.ModelID, revision, paramName, encodedPath)
				utils.Log.Infof("ModelScope Tree API URL: %s (param=%s, try %d)", apiURL, paramName, idx+1)
				resp, err := d.client.R().SetContext(ctx).Get(apiURL)
				if err != nil {
					utils.Log.Errorf("modelscope tree api request error: %+v", err)
					lastErr = err
					continue
				}
				if resp.StatusCode() != http.StatusOK {
					utils.Log.Errorf("modelscope tree api status error: %d, body: %s", resp.StatusCode(), string(resp.Body()))
					lastErr = fmt.Errorf("failed to list files: status code %d", resp.StatusCode())
					continue
				}
				var fileListResp FileListResponse
				if err := json.Unmarshal(resp.Body(), &fileListResp); err != nil {
					utils.Log.Errorf("modelscope tree api unmarshal error: %+v", err)
					utils.Log.Errorf("modelscope tree api response body: %s", string(resp.Body()))
					lastErr = err
					continue
				}
				// Accept either Success==true or Code==200
				if !fileListResp.Success && fileListResp.Code != 200 {
					utils.Log.Errorf("modelscope tree api logic error: %s (RequestId: %s, Code: %d)", fileListResp.Message, fileListResp.RequestID, fileListResp.Code)
					lastErr = fmt.Errorf("modelscope api error: %s", fileListResp.Message)
					continue
				}
				utils.Log.Infof("ModelScope dataset tree entries: %d (path='%s', param=%s)", len(fileListResp.Data.Files), path, paramName)
				// log up to first 5 entries for debugging
				max := 5
				if len(fileListResp.Data.Files) < max {
					max = len(fileListResp.Data.Files)
				}
				for i := 0; i < max; i++ {
					utils.Log.Infof(" - %s (%s)", fileListResp.Data.Files[i].Name, fileListResp.Data.Files[i].Type)
				}
				return d.datasetFilesToObjs(fileListResp.Data.Files, path), nil
			}
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("modelscope dataset tree failed with all candidates")
	}
	return nil, lastErr
}

var _ driver.Driver = (*ModelScope)(nil)

// datasetFilesToObjs constructs objects with driver-root absolute Path combining root and current relative path.
func (d *ModelScope) datasetFilesToObjs(files []File, currentRelPath string) []model.Obj {
	objects := make([]model.Obj, 0, len(files))
	for _, f := range files {
		isDir := f.Type == "tree"
		childRel := f.Name
		if strings.TrimSpace(currentRelPath) != "" {
			childRel = stdpath.Join(currentRelPath, f.Name)
		}
		objects = append(objects, &model.Object{
			Name:     f.Name,
			Path:     childRel, // keep repo-relative path for consistent navigation
			Size:     f.Size,
			Modified: time.Unix(f.CommittedDate, 0),
			IsFolder: isDir,
		})
	}
	return objects
}

// repoRelativePath trims the driver root prefix (ModelID) to produce repo-relative path used by ModelScope APIs.
func (d *ModelScope) repoRelativePath(p string) string {
	raw := strings.TrimSpace(p)
	root := strings.Trim(d.GetRootPath(), "/")
	q := strings.TrimPrefix(raw, "/")
	if q == root {
		return ""
	}
	if strings.HasPrefix(q, root+"/") {
		q = strings.TrimPrefix(q, root+"/")
	}
	return strings.TrimPrefix(q, "/")
}
