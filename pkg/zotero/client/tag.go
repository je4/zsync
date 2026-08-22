package client

import (
	"context"
	"fmt"
	"strconv"

	"github.com/je4/zsync/v2/pkg/zotero/model"
	"gopkg.in/resty.v1"
)

// GetTags returns all tags of the group that changed since sinceVersion,
// together with the group's last modified version.
func (c *Client) GetTags(ctx context.Context, groupId int64, sinceVersion int64) ([]model.Tag, int64, error) {
	endpoint := fmt.Sprintf("/groups/%v/tags", groupId)
	return fetchPaginatedSlices[model.Tag](ctx, c, endpoint, 100, func(req *resty.Request) {
		if sinceVersion > 0 {
			req.SetQueryParam("since", strconv.FormatInt(sinceVersion, 10))
		}
	})
}
