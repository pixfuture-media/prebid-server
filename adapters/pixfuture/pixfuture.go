package pixfuture

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/prebid/openrtb/v20/openrtb2"
	"github.com/prebid/prebid-server/v3/adapters"
	"github.com/prebid/prebid-server/v3/config"
	"github.com/prebid/prebid-server/v3/errortypes"
	"github.com/prebid/prebid-server/v3/openrtb_ext"
	"github.com/prebid/prebid-server/v3/util/jsonutil"
)

type adapter struct {
	endpoint string
}

// Builder builds a new instance of the Pixfuture adapter.
func Builder(bidderName openrtb_ext.BidderName, config config.Adapter, server config.Server) (adapters.Bidder, error) {
	return &adapter{
		endpoint: config.Endpoint,
	}, nil
}

// MakeRequests prepares and serializes HTTP requests to be sent to the Pixfuture endpoint.
func (a *adapter) MakeRequests(bidRequest *openrtb2.BidRequest, reqInfo *adapters.ExtraRequestInfo) ([]*adapters.RequestData, []error) {
	if len(bidRequest.Imp) == 0 {
		return nil, []error{errors.New("no impressions in bid request")}
	}

	// Extract impression IDs
	impIDs := make([]string, len(bidRequest.Imp))
	for i, imp := range bidRequest.Imp {
		impIDs[i] = imp.ID
	}

	// Create the outgoing request
	body, err := jsonutil.Marshal(bidRequest)
	if err != nil {
		return nil, []error{fmt.Errorf("failed to marshal bid request: %w", err)}
	}

	request := &adapters.RequestData{
		Method: http.MethodPost,
		Uri:    a.endpoint,
		Body:   body,
		Headers: http.Header{
			"Content-Type": []string{"application/json"},
		},
		ImpIDs: impIDs,
	}

	return []*adapters.RequestData{request}, nil
}

// getMediaTypeForBid extracts the bid type based on the bid extension data.
func getMediaTypeForBid(bid openrtb2.Bid) (openrtb_ext.BidType, error) {
	if bid.Ext != nil {
		var bidExt openrtb_ext.ExtBid
		err := jsonutil.Unmarshal(bid.Ext, &bidExt)
		if err == nil && bidExt.Prebid != nil {
			return openrtb_ext.ParseBidType(string(bidExt.Prebid.Type))
		}
	}

	return "", &errortypes.BadServerResponse{
		Message: fmt.Sprintf("Failed to parse impression \"%s\" mediatype", bid.ImpID),
	}
}

// MakeBids parses the HTTP response from the Pixfuture endpoint and generates a BidderResponse.
func (a *adapter) MakeBids(request *openrtb2.BidRequest, requestData *adapters.RequestData, responseData *adapters.ResponseData) (*adapters.BidderResponse, []error) {
	// Handle No Content response
	if adapters.IsResponseStatusCodeNoContent(responseData) {
		return nil, nil
	}

	// Check for errors in response status code
	if err := adapters.CheckResponseStatusCodeForErrors(responseData); err != nil {
		return nil, []error{err}
	}

	// Parse the response body
	var response openrtb2.BidResponse
	if err := jsonutil.Unmarshal(responseData.Body, &response); err != nil {
		return nil, []error{err}
	}

	bidResponse := adapters.NewBidderResponseWithBidsCapacity(len(request.Imp))
	bidResponse.Currency = response.Cur
	var errors []error

	for _, seatBid := range response.SeatBid {
		for i, bid := range seatBid.Bid {
			bidType, err := getMediaTypeForBid(bid)
			if err != nil {
				errors = append(errors, err)
				continue
			}

			bidResponse.Bids = append(bidResponse.Bids, &adapters.TypedBid{
				Bid:     &seatBid.Bid[i],
				BidType: bidType,
			})
		}
	}

	return bidResponse, errors
}
