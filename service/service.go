package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"net/http"
	"net/url"
	"os"
	"path"

	"github.com/anacrolix/torrent/metainfo"
)

type UploadOptions struct {
	url.Values
}

// NewUploadOptions wraps url.Values and returns a new UploadOptions initialized with values from the http.Request.
func NewUploadOptions(r *http.Request) UploadOptions {
	uo := UploadOptions{url.Values{}}
	uo.AddFromRequest(r, "title")
	uo.AddFromRequest(r, "description")

	return uo
}

func (uo *UploadOptions) AddFromRequest(r *http.Request, name string) {
	val := r.URL.Query().Get(name)
	if val != "" {
		uo.Add(name, val)
	}
}

type ServiceClient struct {
	// This should be a URL to handle uploads. The specifics are in replica-rust.
	ReplicaServiceEndpoint func() *url.URL
	HttpClient             *http.Client
}

func (cl ServiceClient) Upload(read io.Reader, fileName string, uploadOptions *UploadOptions) (output UploadOutput, err error) {
	req, err := http.NewRequest(http.MethodPut, serviceUploadUrl(cl.ReplicaServiceEndpoint, fileName).String(), read)
	if err != nil {
		err = fmt.Errorf("creating put request: %w", err)
		return
	}

	req.URL.RawQuery = uploadOptions.Encode()

	req.Header.Set("Accept", "application/json, text/plain, text/html;q=0")
	resp, err := cl.HttpClient.Do(req)
	if err != nil {
		err = fmt.Errorf("doing request: %w", err)
		return
	}
	defer resp.Body.Close()
	respBodyBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		err = fmt.Errorf("reading all response body bytes: %w", err)
		return
	}
	if resp.StatusCode != http.StatusOK {
		err = fmt.Errorf("got unexpected status code %v for response %q",
			resp.StatusCode, respBodyBytes)
		return
	}
	var serviceOutput ServiceUploadOutput
	err = json.Unmarshal(respBodyBytes, &serviceOutput)
	if err != nil {
		err = fmt.Errorf("decoding response %q: %w", string(respBodyBytes), err)
		return
	}
	output.Link = &serviceOutput.Link
	output.AuthToken = &serviceOutput.AdminToken
	var metainfoBytesBuffer bytes.Buffer
	for _, r := range serviceOutput.Metainfo {
		if r < 0 || r > math.MaxUint8 {
			err = fmt.Errorf("response metainfo rune has unexpected codepoint")
			return
		}
		err = metainfoBytesBuffer.WriteByte(byte(r))
		if err != nil {
			panic(err)
		}
	}
	mi, err := metainfo.Load(&metainfoBytesBuffer)
	if err != nil {
		err = fmt.Errorf("parsing metainfo from response: %w", err)
		return
	}
	output.MetaInfo = mi
	output.Info, err = mi.UnmarshalInfo()
	if err != nil {
		err = fmt.Errorf("unmarshalling info from response metainfo bytes: %w", err)
		return
	}
	m, err := metainfo.ParseMagnetURI(serviceOutput.Link)
	if err != nil {
		err = fmt.Errorf("parsing response replica link: %w", err)
		return
	}
	err = output.Upload.FromMagnet(m)
	if err != nil {
		err = fmt.Errorf("extracting upload specifics from response replica link: %w", err)
		return
	}
	return
}

// UploadFile uploads the file for the given name, returning the Replica magnet link for the upload.
func (cl ServiceClient) UploadFile(fileName, uploadedAsName string, uploadOptions *UploadOptions) (_ UploadOutput, err error) {
	f, err := os.Open(fileName)
	if err != nil {
		err = fmt.Errorf("opening file: %w", err)
		return
	}
	defer f.Close()
	return cl.Upload(f, uploadedAsName, uploadOptions)
}

func (cl ServiceClient) DeleteUpload(prefix Prefix, auth string, haveMetainfo bool) error {
	data := url.Values{
		"prefix": {prefix.PrefixString()},
		"auth":   {auth},
	}
	if haveMetainfo {
		// We only use this field if we need to, to prevent detection and for backward compatibility.
		data["have_metainfo"] = []string{"true"}
	}
	resp, err := cl.HttpClient.PostForm(
		serviceDeleteUrl(cl.ReplicaServiceEndpoint).String(),
		data,
	)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected response: %q", resp.Status)
	}
	return nil
}

// This exists for anything that doesn't have configuration but expects to connect to an arbitrary
// replica-rust service. At least in flashlight, this is provided by configuration instead.
var GlobalChinaDefaultServiceUrl = &url.URL{
	Scheme: "https",
	Host:   "replica-search.lantern.io",
}

// Interface to the replica-rust/"Replica service".

type ServiceUploadOutput struct {
	Link       string `json:"link"`
	Metainfo   string `json:"metainfo"`
	AdminToken string `json:"admin_token"`
}

// Completes the upload endpoint URL with the file-name, per the replica-rust upload endpoint API.
func serviceUploadUrl(base func() *url.URL, fileName string) *url.URL {
	return base().ResolveReference(&url.URL{Path: path.Join("upload", fileName)})
}

func serviceDeleteUrl(base func() *url.URL) *url.URL {
	return base().ResolveReference(&url.URL{Path: "delete"})
}
