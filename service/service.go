package service

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"net/http"
	"net/url"
	"os"
	"path"

	"github.com/anacrolix/torrent/bencode"
	"github.com/anacrolix/torrent/metainfo"
)

type UploadOptions struct {
	Title       string
	Description string
}

// NewUploadOptions returns a new UploadOptions initialized with values from the http.Request.
func NewUploadOptions(r *http.Request) UploadOptions {
	uo := UploadOptions{}

	q := r.URL.Query()
	uo.Title = q.Get("title")
	uo.Description = q.Get("description")

	return uo
}

func (uo *UploadOptions) Encode() string {
	v := url.Values{}

	if uo.Title != "" {
		v.Add("title", uo.Title)
	}

	if uo.Description != "" {
		v.Add("description", uo.Description)
	}

	return v.Encode()
}

type ServiceClient struct {
	// This should be a URL to handle uploads. The specifics are in replica-rust.
	ReplicaServiceEndpoint func() *url.URL
	HttpClient             *http.Client
}

func (cl ServiceClient) Upload(read io.Reader, fileName string, uploadOptions UploadOptions) (output UploadOutput, err error) {
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
	var mi metainfo.MetaInfo
	err = bencode.Unmarshal(serviceOutput.Metainfo.Bytes, &mi)
	if err != nil {
		err = fmt.Errorf("parsing metainfo from response: %w", err)
		return
	}
	output.MetaInfo = &mi
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
func (cl ServiceClient) UploadFile(fileName, uploadedAsName string, uploadOptions UploadOptions) (_ UploadOutput, err error) {
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
	Link       string           `json:"link"`
	Metainfo   JsonBinaryString `json:"metainfo"`
	AdminToken string           `json:"admin_token"`
}

// JsonBinaryString tunnels binary data through a UTF-8 JSON string.
type JsonBinaryString struct {
	// This is embedded to break existing interactions with a string type.
	Bytes []byte
}

func (me JsonBinaryString) MarshalJSON() ([]byte, error) {
	var s []rune
	for _, b := range me.Bytes {
		s = append(s, rune(b))
	}
	return json.Marshal(string(s))
}

func (me *JsonBinaryString) UnmarshalJSON(i []byte) error {
	var s string
	err := json.Unmarshal(i, &s)
	if err != nil {
		return err
	}
	me.Bytes = me.Bytes[:0]
	for _, r := range s {
		if r < 0 || r > math.MaxUint8 {
			return fmt.Errorf("rune out of bounds for byte: %q", r)
		}
		me.Bytes = append(me.Bytes, byte(r))
	}
	return nil
}

var (
	_ json.Unmarshaler = (*JsonBinaryString)(nil)
	_ json.Marshaler   = JsonBinaryString{}
)

// Completes the upload endpoint URL with the file-name, per the replica-rust upload endpoint API.
func serviceUploadUrl(base func() *url.URL, fileName string) *url.URL {
	return base().ResolveReference(&url.URL{Path: path.Join("upload", fileName)})
}

func serviceDeleteUrl(base func() *url.URL) *url.URL {
	return base().ResolveReference(&url.URL{Path: "delete"})
}
