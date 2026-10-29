package upload

import (
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/qubernetic/copia-cli/pkg/cmdutil"
	"github.com/qubernetic/copia-cli/pkg/iostreams"
)

type UploadOptions struct {
	IO         *iostreams.IOStreams
	HTTPClient *http.Client
	Host       string
	Token      string
	Owner      string
	Repo       string
	Tag        string
	Files      []string
}

func NewCmdUpload(f *cmdutil.Factory) *cobra.Command {
	opts := &UploadOptions{}

	cmd := &cobra.Command{
		Use:   "upload <tag> <files>...",
		Short: "Upload assets to a release",
		Long:  "Upload asset files to a Copia release. Specify the release tag followed by one or more file paths.",
		Example: `  # Upload a single asset
  $ copia-cli release upload v1.0.0 binary.tar.gz

  # Upload multiple assets
  $ copia-cli release upload v1.0.0 *.tar.gz *.zip`,
		Args: cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Tag = args[0]
			opts.Files = args[1:]
			opts.IO = f.IOStreams

			host, token, err := f.ResolveAuth()
			if err != nil {
				return err
			}
			opts.Host = host
			opts.Token = token

			owner, repo, err := f.ResolveRepo()
			if err != nil {
				return err
			}
			opts.Owner = owner
			opts.Repo = repo
			opts.HTTPClient = cmdutil.NewHTTPClient()
			return UploadRun(opts)
		},
	}

	return cmd
}

func UploadRun(opts *UploadOptions) error {
	// Validate files exist first
	for _, f := range opts.Files {
		if _, err := os.Stat(f); err != nil {
			return fmt.Errorf("opening %s: %w", f, err)
		}
	}

	// Look up release ID by tag
	lookupURL := fmt.Sprintf("https://%s/api/v1/repos/%s/%s/releases/tags/%s",
		opts.Host, opts.Owner, opts.Repo, opts.Tag)

	lookupReq, err := http.NewRequest("GET", lookupURL, nil)
	if err != nil {
		return err
	}
	lookupReq.Header.Set("Authorization", "token "+opts.Token)

	lookupResp, err := opts.HTTPClient.Do(lookupReq)
	if err != nil {
		return fmt.Errorf("connecting to %s: %w", opts.Host, err)
	}
	defer func() { _ = lookupResp.Body.Close() }()

	if lookupResp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("release %s not found", opts.Tag)
	}

	lookupBody, err := io.ReadAll(lookupResp.Body)
	if err != nil {
		return err
	}

	var release struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(lookupBody, &release); err != nil {
		return err
	}

	// Upload each file
	for _, filePath := range opts.Files {
		if err := uploadFile(opts, release.ID, filePath); err != nil {
			return fmt.Errorf("uploading %s: %w", filepath.Base(filePath), err)
		}
		_, _ = fmt.Fprintf(opts.IO.Out, "Uploaded %s to %s\n", filepath.Base(filePath), opts.Tag)
	}

	return nil
}

func uploadFile(opts *UploadOptions, releaseID int64, filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	pr, pw := io.Pipe()
	writer := multipart.NewWriter(pw)

	go func() {
		part, err := writer.CreateFormFile("attachment", filepath.Base(filePath))
		if err != nil {
			_ = pw.CloseWithError(err)
			return
		}
		if _, err := io.Copy(part, file); err != nil {
			_ = pw.CloseWithError(err)
			return
		}
		_ = writer.Close()
		_ = pw.Close()
	}()

	url := fmt.Sprintf("https://%s/api/v1/repos/%s/%s/releases/%d/assets",
		opts.Host, opts.Owner, opts.Repo, releaseID)

	req, err := http.NewRequest("POST", url, pr)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "token "+opts.Token)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := opts.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	return nil
}
