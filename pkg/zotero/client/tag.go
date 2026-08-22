package client

import (
	"fmt"
	"strconv"

	"github.com/je4/zsync/v2/pkg/zotero/model"
	"gopkg.in/resty.v1"
)

func (c *Client) GetTagsVersion(groupId int64, sinceVersion int64) (*[]model.Tag, int64, error) {
	endpoint := fmt.Sprintf("/groups/%v/tags", groupId)
	tags, lmv, err := fetchPaginatedSlices[model.Tag](c, endpoint, 100, func(req *resty.Request) {
		if sinceVersion > 0 {
			req.SetQueryParam("since", strconv.FormatInt(sinceVersion, 10))
		}
	})
	if err != nil {
		return nil, 0, err
	}
	return &tags, lmv, nil
}
