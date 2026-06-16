package modelscope

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	neturl "net/url"
	stdpath "path"
	"strings"
	"sync"
	"time"

	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/go-resty/resty/v2"
)

const (
	apiEndpoint         = "https://www.modelscope.cn"
	resourceTypeModel   = "model"
	resourceTypeDataset = "dataset"
	requestTimeout      = 30 * time.Second
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
	d.client = resty.New().SetTimeout(requestTimeout)
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

	if d.ResourceType == resourceTypeDataset {
		return d.listDatasetTree(ctx, path)
	}

	return d.tryAllCandidates(ctx, func(ctx context.Context, segment, revision string) ([]model.Obj, bool) {
		getURL := fmt.Sprintf("%s/api/v1/%s/%s/repo/files?Revision=%s&Recursive=false&Root=%s", apiEndpoint, segment, d.ModelID, revision, path)
		resp, err := d.client.R().SetContext(ctx).Get(getURL)
		if err == nil && resp.StatusCode() == http.StatusOK {
			var filesResp FileListResponse
			if uerr := json.Unmarshal(resp.Body(), &filesResp); uerr == nil && (filesResp.Success || filesResp.Code == 200) {
				return filesToObjs(filesResp.Data.Files), true
			}
		}
		if resp != nil && (resp.StatusCode() == http.StatusMethodNotAllowed || resp.StatusCode() == http.StatusNotFound) {
			postURL := fmt.Sprintf("%s/api/v1/%s/%s/repo/files", apiEndpoint, segment, d.ModelID)
			payload := map[string]interface{}{"Revision": revision, "Recursive": false, "Root": path}
			resp2, err2 := d.client.R().SetHeader("Content-Type", "application/json").SetBody(payload).SetContext(ctx).Post(postURL)
			if err2 == nil && resp2.StatusCode() == http.StatusOK {
				var filesResp2 FileListResponse
				if uerr := json.Unmarshal(resp2.Body(), &filesResp2); uerr == nil && (filesResp2.Success || filesResp2.Code == 200) {
					return filesToObjs(filesResp2.Data.Files), true
				}
			}
		}
		return nil, false
	})
}

func (d *ModelScope) Link(ctx context.Context, file model.Obj, args model.LinkArgs) (*model.Link, error) {
	filePath := file.GetPath()
	if d.ResourceType == resourceTypeDataset {
		filePath = d.repoRelativePath(filePath)
	}

	return d.tryAllLinkCandidates(ctx, func(ctx context.Context, segment, revision string) (*model.Link, bool) {
		client := resty.New().SetRedirectPolicy(resty.NoRedirectPolicy()).SetTimeout(requestTimeout)
		encodedFilePath := neturl.QueryEscape(filePath)
		apiURL := fmt.Sprintf("%s/api/v1/%s/%s/repo?Revision=%s&FilePath=%s", apiEndpoint, segment, d.ModelID, revision, encodedFilePath)

		resp, err := client.R().SetContext(ctx).Get(apiURL)
		if err == nil && resp != nil {
			switch resp.StatusCode() {
			case http.StatusFound:
				if finalURL := resp.Header().Get("Location"); finalURL != "" {
					if args.Redirect {
						return &model.Link{URL: finalURL}, true
					}
					return &model.Link{URL: apiURL}, true
				}
			case http.StatusOK:
				return &model.Link{URL: apiURL}, true
			}
		}

		status := 0
		if resp != nil {
			status = resp.StatusCode()
		}
		if status == http.StatusMethodNotAllowed || status == http.StatusNotFound || err != nil {
			postURL := fmt.Sprintf("%s/api/v1/%s/%s/repo", apiEndpoint, segment, d.ModelID)
			payload := map[string]interface{}{"Revision": revision, "FilePath": filePath}
			resp2, err2 := client.R().SetHeader("Content-Type", "application/json").SetBody(payload).SetContext(ctx).Post(postURL)
			if err2 == nil && resp2 != nil {
				switch resp2.StatusCode() {
				case http.StatusFound:
					if finalURL := resp2.Header().Get("Location"); finalURL != "" {
						if args.Redirect {
							return &model.Link{URL: finalURL}, true
						}
						return &model.Link{URL: postURL}, true
					}
				case http.StatusOK:
					return &model.Link{URL: postURL}, true
				}
			}
		}
		return nil, false
	})
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

// tryAllCandidates tries the primary (segment[0] × revision[0]) first.
// If that succeeds, return immediately. Otherwise probe all other
// combinations concurrently and return the first success.
func (d *ModelScope) tryAllCandidates(
	ctx context.Context,
	probe func(ctx context.Context, segment, revision string) ([]model.Obj, bool),
) ([]model.Obj, error) {
	segments := d.resourceSegmentsCandidates()
	revisions := d.revisionCandidates()

	// Fast path: primary candidate
	if result, ok := probe(ctx, segments[0], revisions[0]); ok {
		return result, nil
	}

	// Probe alternatives concurrently
	var (
		mu     sync.Mutex
		result []model.Obj
		found  bool
	)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var wg sync.WaitGroup
	for _, seg := range segments {
		for _, rev := range revisions {
			if seg == segments[0] && rev == revisions[0] {
				continue // already tried
			}
			wg.Add(1)
			go func(segment, revision string) {
				defer wg.Done()
				if r, ok := probe(ctx, segment, revision); ok {
					mu.Lock()
					if !found {
						result = r
						found = true
						cancel()
					}
					mu.Unlock()
				}
			}(seg, rev)
		}
	}
	wg.Wait()

	if found {
		return result, nil
	}
	return nil, fmt.Errorf("modelscope list failed with all strategies")
}

// tryAllLinkCandidates is the Link variant of tryAllCandidates.
func (d *ModelScope) tryAllLinkCandidates(
	ctx context.Context,
	probe func(ctx context.Context, segment, revision string) (*model.Link, bool),
) (*model.Link, error) {
	segments := d.resourceSegmentsCandidates()
	revisions := d.revisionCandidates()

	// Fast path: primary candidate
	if link, ok := probe(ctx, segments[0], revisions[0]); ok {
		return link, nil
	}

	// Probe alternatives concurrently
	var (
		mu     sync.Mutex
		result *model.Link
		found  bool
	)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var wg sync.WaitGroup
	for _, seg := range segments {
		for _, rev := range revisions {
			if seg == segments[0] && rev == revisions[0] {
				continue
			}
			wg.Add(1)
			go func(segment, revision string) {
				defer wg.Done()
				if link, ok := probe(ctx, segment, revision); ok {
					mu.Lock()
					if !found {
						result = link
						found = true
						cancel()
					}
					mu.Unlock()
				}
			}(seg, rev)
		}
	}
	wg.Wait()

	if found {
		return result, nil
	}
	return nil, fmt.Errorf("modelscope link failed with all strategies")
}

func (d *ModelScope) listDatasetTree(ctx context.Context, relPath string) ([]model.Obj, error) {
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

	return d.tryAllCandidates(ctx, func(ctx context.Context, segment, revision string) ([]model.Obj, bool) {
		encodedPath := neturl.QueryEscape(path)
		for _, paramName := range []string{"Root", "Path"} {
			apiURL := fmt.Sprintf("%s/api/v1/%s/%s/repo/tree?Revision=%s&%s=%s&PageNumber=1&PageSize=1000", apiEndpoint, segment, d.ModelID, revision, paramName, encodedPath)
			resp, err := d.client.R().SetContext(ctx).Get(apiURL)
			if err != nil {
				if ctx.Err() != nil {
					return nil, false
				}
				continue
			}
			if resp.StatusCode() != http.StatusOK {
				continue
			}
			var filesResp FileListResponse
			if uerr := json.Unmarshal(resp.Body(), &filesResp); uerr != nil {
				continue
			}
			if !filesResp.Success && filesResp.Code != 200 {
				continue
			}
			return d.datasetFilesToObjs(filesResp.Data.Files, path), true
		}
		return nil, false
	})
}

var _ driver.Driver = (*ModelScope)(nil)

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
			Path:     childRel,
			Size:     f.Size,
			Modified: time.Unix(f.CommittedDate, 0),
			IsFolder: isDir,
		})
	}
	return objects
}

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
