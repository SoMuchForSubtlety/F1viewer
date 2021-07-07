package f1tv

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"github.com/SoMuchForSubtlety/f1viewer/v2/internal/util"
)

const (
	baseURL = "https://f1tv.formula1.com"
	authURL = "https://api.formula1.com/v2/account/subscriber/authenticate/by-password"

	playbackRequestPath            = "/1.0/R/ENG/%v/ALL/CONTENT/PLAY?contentId=%d"
	playbackPerspectiveRequestPath = "/1.0/R/ENG/%v/ALL/%s"
	contentDetailsPath             = "/2.0/R/ENG/%v/ALL/CONTENT/VIDEO/%d/F1_TV_Pro_Annual/14"
	categoryPagePath               = "/2.0/R/ENG/%v/ALL/PAGE/%v/F1_TV_Pro_Annual/2"

	apiKey = "fCUCjWrKPu9ylJwRAv8BpGLEgiAuThx7"

	BIG_SCREEN_HLS  StreamType = "BIG_SCREEN_HLS"
	WEB_HLS         StreamType = "WEB_HLS"
	TABLET_HLS      StreamType = "TABLET_HLS"
	MOBILE_HLS      StreamType = "MOBILE_HLS"
	BIG_SCREEN_DASH StreamType = "BIG_SCREEN_DASH"
	WEB_DASH        StreamType = "WEB_DASH"
	MOBILE_DASH     StreamType = "MOBILE_DASH"
	TABLET_DASH     StreamType = "TABLET_DASH"

	PAGE_HOMEPAGE      PageID = 395
	PAGE_ARCHIVE       PageID = 493
	PAGE_SHOWS         PageID = 410
	PAGE_DOCUMENTARIES PageID = 413
	PAGE_SEASON_20201  PageID = 1510

	VIDEO    ContentType = "VIDEO"
	BUNDLE   ContentType = "BUNDLE"
	LAUNCHER ContentType = "LAUNCHER"

	LIVE   ContentSubType = "LIVE"
	REPLAY ContentSubType = "REPLAY"
)

type ContentType string

type ContentSubType string

type StreamType string

type PageID int64

func assembleURL(urlPath string, format StreamType, args ...interface{}) (*url.URL, error) {
	args = append([]interface{}{format}, args...)
	return url.Parse(baseURL + fmt.Sprintf(urlPath, args...))
}

type F1TV struct {
	SubscriptionToken string
	userAgent         string
	Client            *http.Client
}

func NewF1TV(version string) *F1TV {
	return &F1TV{
		userAgent: fmt.Sprintf("f1viewer/%s (%s)", version, runtime.GOOS),
		Client:    http.DefaultClient,
	}
}

func (f *F1TV) Authenticate(username, password string, logger util.Logger) error {
	type request struct {
		Login    string `json:"Login"`
		Password string `json:"Password"`
	}

	payloadBuf := new(bytes.Buffer)
	err := json.NewEncoder(payloadBuf).Encode(request{Login: username, Password: password})
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, authURL, payloadBuf)
	req.Header.Set("apiKey", apiKey)
	req.Header.Set("User-Agent", "RaceControl f1viewer")
	if err != nil {
		return err
	}

	resp, err := f.Client.Do(req)
	if err != nil {
		return err
	}
	var auth struct {
		Data struct {
			SubscriptionStatus string `json:"subscriptionStatus"`
			SubscriptionToken  string `json:"subscriptionToken"`
		} `json:"data"`
	}

	err = json.NewDecoder(resp.Body).Decode(&auth)
	logger.Infof("subscription status: %s", auth.Data.SubscriptionStatus)
	if auth.Data.SubscriptionToken == "" {
		return errors.New("could not get subscription token")
	}
	f.SubscriptionToken = auth.Data.SubscriptionToken
	return err
}

func (f *F1TV) GetContent(format StreamType, category PageID, v interface{}) error {
	reqURL, err := assembleURL(categoryPagePath, format, category)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodGet, reqURL.String(), nil)
	if err != nil {
		return err
	}
	resp, err := f.Client.Do(req)
	if err != nil {
		return fmt.Errorf("error during request: %w", err)
	}

	return json.NewDecoder(resp.Body).Decode(v)
}

type RemoteContent struct {
	ID    PageID
	Title string
}

func (f *F1TV) GetPageContent(id PageID) ([]TopContainer, []RemoteContent, error) {
	var resp APIResponse
	err := f.GetContent(WEB_DASH, id, &resp)
	if err != nil {
		return nil, nil, err
	}

	var content []TopContainer
	var bundles []RemoteContent
	for _, container := range resp.ResultObj.Containers {
		var videoContainers []ContentContainer
		for _, contentContainer := range container.RetrieveItems.ResultObj.Containers {
			switch contentContainer.Metadata.ContentType {
			case VIDEO:
				videoContainers = append(videoContainers, contentContainer)
			case BUNDLE:
				if contentContainer.Metadata.EmfAttributes.PageID == id {
					// we don't need recusion
					continue
				}
				title := contentContainer.Metadata.Label
				if title == "" {
					title = contentContainer.Metadata.TitleBrief
				}
				if title == "" {
					title = contentContainer.Metadata.EmfAttributes.GlobalTitle
				}
				if title == "" {
					title = contentContainer.Metadata.EmfAttributes.GlobalMeetingName
				}
				bundles = append(bundles, RemoteContent{ID: contentContainer.Metadata.EmfAttributes.PageID, Title: title})
			case LAUNCHER:
				if len(contentContainer.Actions) == 0 || contentContainer.Actions[0].HREF == "" {
					continue
				}
				title := contentContainer.Metadata.Title
				idString := strings.Split(contentContainer.Actions[0].HREF, "/")[2]
				id, err := strconv.ParseInt(idString, 10, 64)
				if err != nil {
					continue
				}
				bundles = append(bundles, RemoteContent{ID: PageID(id), Title: title})
			}
		}
		container.RetrieveItems.ResultObj.Containers = videoContainers
		if len(videoContainers) > 0 {
			content = append(content, container)
		}
		sort.Slice(bundles, func(i, j int) bool { return bundles[i].Title > bundles[j].Title })
	}

	return content, bundles, err
}

func (s AdditionalStream) PrettyName() string {
	switch s.Title {
	case "PIT LANE":
		return "Pit Lane"
	case "TRACKER":
		return "Driver Tracker"
	case "DATA":
		return "Data Channel"
	default:
		return fmt.Sprintf("%s %s", s.DriverFirstName, s.DriverLastName)
	}
}

func (f *F1TV) GetLiveVideoContainers() ([]ContentContainer, error) {
	topContainers, _, err := f.GetPageContent(PAGE_HOMEPAGE)
	if err != nil {
		return nil, err
	}
	var live []ContentContainer
	ids := make(map[int64]struct{})
	for _, vidContainers := range topContainers {
		for _, v := range vidContainers.RetrieveItems.ResultObj.Containers {
			_, ok := ids[v.Metadata.ContentID]
			if !ok && v.Metadata.ContentSubtype == LIVE {
				ids[v.Metadata.ContentID] = struct{}{}
				live = append(live, v)
			}
		}
	}

	return live, nil
}

func (f *F1TV) ContentDetails(contentID int64) (TopContainer, error) {
	reqURL, err := assembleURL(contentDetailsPath, BIG_SCREEN_HLS, contentID)
	if err != nil {
		return TopContainer{}, err
	}
	req, err := http.NewRequest(http.MethodGet, reqURL.String(), nil)
	if err != nil {
		return TopContainer{}, err
	}

	resp, err := f.Client.Do(req)
	if err != nil {
		return TopContainer{}, err
	}
	defer resp.Body.Close()

	var details APIResponse
	err = json.NewDecoder(resp.Body).Decode(&details)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return TopContainer{}, fmt.Errorf("got status code %d: %s", resp.StatusCode, details.Message)
	}

	if len(details.ResultObj.Containers) == 0 {
		return TopContainer{}, fmt.Errorf("no content details for %d", contentID)
	}
	return details.ResultObj.Containers[0], err
}

func (f *F1TV) GetPerspectivePlaybackURL(format StreamType, path string) (string, error) {
	reqURL, err := assembleURL(playbackPerspectiveRequestPath, format, path)
	if err != nil {
		return "", nil
	}

	return f.playbackURL(reqURL.String())
}

func (f *F1TV) GetPlaybackURL(format StreamType, contentID int64) (string, error) {
	reqURL, err := assembleURL(playbackRequestPath, format, contentID)
	if err != nil {
		return "", nil
	}

	return f.playbackURL(reqURL.String())
}

func (f *F1TV) playbackURL(reqURL string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		return "", nil
	}

	req.Header.Set("ascendontoken", f.SubscriptionToken)
	httpResp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}

	var resp struct {
		ResultCode       string `json:"resultCode"`
		Message          string `json:"message"`
		ErrorDescription string `json:"errorDescription"`
		ResultObj        struct {
			EntitlementToken string `json:"entitlementToken"`
			URL              string `json:"url"`
			StreamType       string `json:"streamType"`
		} `json:"resultObj"`
		SystemTime int64 `json:"systemTime"`
	}

	err = json.NewDecoder(httpResp.Body).Decode(&resp)
	if err != nil {
		return "", err
	}

	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		err = errors.New(resp.Message)
	} else if resp.ResultObj.URL == "" {
		err = fmt.Errorf("API returned empty URL: %s", resp.Message)
	}

	return resp.ResultObj.URL, err
}
