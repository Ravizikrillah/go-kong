package kong

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// AbstractTargetService handles Targets in Kong.
type AbstractTargetService interface {
	// Create creates a Target in Kong under upstreamID.
	Create(ctx context.Context, upstreamNameOrID *string, target *Target) (*Target, error)
	// Delete deletes a Target in Kong
	Delete(ctx context.Context, upstreamNameOrID *string, targetOrID *string) error
	// List fetches a list of Targets in Kong.
	List(ctx context.Context, upstreamNameOrID *string, opt *ListOpt) ([]*Target, *ListOpt, error)
	// ListAll fetches all Targets in Kong for an upstream.
	ListAll(ctx context.Context, upstreamNameOrID *string) ([]*Target, error)
	// MarkHealthy marks target belonging to upstreamNameOrID as healthy in
	// Kong's load balancer.
	MarkHealthy(ctx context.Context, upstreamNameOrID *string, target *Target) error
	// MarkUnhealthy marks target belonging to upstreamNameOrID as unhealthy in
	// Kong's load balancer.
	MarkUnhealthy(ctx context.Context, upstreamNameOrID *string, target *Target) error
}

// TargetService handles Targets in Kong.
type TargetService service

// TODO foreign key can be read directly from the embedded key itself
// upstreamNameOrID need not be an explicit parameter.

// Create creates a Target in Kong under upstreamID.
// If an ID is specified, it will be used to
// create a target in Kong, otherwise an ID
// is auto-generated.
func (s *TargetService) Create(ctx context.Context,
	upstreamNameOrID *string, target *Target,
) (*Target, error) {
	if isEmptyString(upstreamNameOrID) {
		return nil, fmt.Errorf("upstreamNameOrID can not be nil")
	}
	queryPath := "/upstreams/" + *upstreamNameOrID + "/targets"
	return makeRequest(ctx, s.client, "POST", queryPath, target)
}

// Delete deletes a Target in Kong
func (s *TargetService) Delete(ctx context.Context,
	upstreamNameOrID *string, targetOrID *string,
) error {
	if isEmptyString(upstreamNameOrID) {
		return fmt.Errorf("upstreamNameOrID cannot be nil for Get operation")
	}
	if isEmptyString(targetOrID) {
		return fmt.Errorf("targetOrID cannot be nil for Delete operation")
	}

	endpoint := fmt.Sprintf("/upstreams/%v/targets/%v",
		*upstreamNameOrID, *targetOrID)
	req, err := s.client.NewRequest("DELETE", endpoint, nil, nil)
	if err != nil {
		return err
	}

	_, err = s.client.Do(ctx, req, nil)
	return err
}

// List fetches a list of Targets in Kong.
// opt can be used to control pagination.
func (s *TargetService) List(ctx context.Context,
	upstreamNameOrID *string, opt *ListOpt,
) ([]*Target, *ListOpt, error) {
	if isEmptyString(upstreamNameOrID) {
		return nil, nil, fmt.Errorf(
			"upstreamNameOrID cannot be nil for Get operation")
	}
	data, next, err := s.client.list(ctx,
		"/upstreams/"+*upstreamNameOrID+"/targets", opt)
	if err != nil {
		return nil, nil, err
	}
	var targets []*Target
	for _, object := range data {
		b, err := object.MarshalJSON()
		if err != nil {
			return nil, nil, err
		}
		var target Target
		err = json.Unmarshal(b, &target)
		if err != nil {
			return nil, nil, err
		}
		targets = append(targets, &target)
	}

	return targets, next, nil
}

// ListAll fetches all Targets in Kong for an upstream.
func (s *TargetService) ListAll(ctx context.Context,
	upstreamNameOrID *string,
) ([]*Target, error) {
	var targets, data []*Target
	var err error
	opt := &ListOpt{Size: pageSize}

	for opt != nil {
		data, opt, err = s.List(ctx, upstreamNameOrID, opt)
		if err != nil {
			return nil, err
		}
		targets = append(targets, data...)
	}
	return targets, nil
}

// MarkHealthy marks target belonging to upstreamNameOrID as healthy in
// Kong's load balancer.
func (s *TargetService) MarkHealthy(ctx context.Context,
	upstreamNameOrID *string, target *Target,
) error {
	if target == nil {
		return fmt.Errorf("cannot set health status for a nil target")
	}
	if isEmptyString(target.ID) && isEmptyString(target.Target) {
		return fmt.Errorf("need at least one of target or ID to" +
			" set health status")
	}
	if isEmptyString(upstreamNameOrID) {
		return fmt.Errorf("upstreamNameOrID cannot be nil " +
			"for updating health check")
	}

	tid := target.ID
	if target.ID == nil {
		tid = target.Target
	}

	endpoint := fmt.Sprintf("/upstreams/%v/targets/%v/healthy",
		*upstreamNameOrID, *tid)
	_, err := makeRequest(ctx, s.client, "POST", endpoint, nil)
	return err
}

// MarkUnhealthy marks target belonging to upstreamNameOrID as unhealthy in
// Kong's load balancer.
func (s *TargetService) MarkUnhealthy(ctx context.Context,
	upstreamNameOrID *string, target *Target,
) error {
	if target == nil {
		return fmt.Errorf("cannot set health status for a nil target")
	}
	if isEmptyString(target.ID) && isEmptyString(target.Target) {
		return fmt.Errorf("need at least one of target or ID to" +
			" set health status")
	}
	if isEmptyString(upstreamNameOrID) {
		return fmt.Errorf("upstreamNameOrID cannot be nil " +
			"for updating health check")
	}

	tid := target.ID
	if target.ID == nil {
		tid = target.Target
	}

	endpoint := fmt.Sprintf("/upstreams/%v/targets/%v/unhealthy",
		*upstreamNameOrID, *tid)
	_, err := makeRequest(ctx, s.client, "POST", endpoint, nil)
	return err
}

func makeRequest(
	ctx context.Context, client *Client, method, endpoint string, target *Target,
) (*Target, error) {
	var err error
	var res *Response
	var req *http.Request

	if target == nil {
		req, err = client.NewRequest(method, endpoint, nil, nil)
	} else {
		req, err = client.NewRequest(method, endpoint, nil, target)
	}
	if err != nil {
		return nil, err
	}

	var createdTarget Target
	if target == nil {
		res, err = client.Do(ctx, req, nil)
	} else {
		res, err = client.Do(ctx, req, &createdTarget)
	}

	// In Kong 3.0 POST requests will not be supported anymore for this entity,
	// so we need to introduce some retry logic to make sure this
	// method works for both POSTs and PUTs
	if err != nil && (res != nil && res.StatusCode == http.StatusMethodNotAllowed) {
		var createdTargetP *Target
		createdTargetP, err = makeRequest(ctx, client, "PUT", endpoint, target)
		createdTarget = *createdTargetP
	}
	if err != nil {
		return nil, err
	}
	return &createdTarget, nil
}
