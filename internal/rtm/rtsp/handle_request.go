/*
 * Copyright (c) 2022-2022 Cisco and/or its affiliates.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at:
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package rtsp

import (
	"errors"
	"github.com/aler9/gortsplib/pkg/base"
	"github.com/aler9/gortsplib/pkg/liberrors"
	"github.com/media-streaming-mesh/msm-cp/internal/model"
)

func (r *RTSP) handleRequest(req *base.Request, connectionKey model.ConnectionKey) (*base.Response, error) {

	var res *base.Response
	var err error
	var ok bool
	var cSeq base.HeaderValue

	if cSeq, ok = req.Header["CSeq"]; !ok || len(cSeq) != 1 {
		r.logError("CSeq missing")

		return &base.Response{
			StatusCode: base.StatusBadRequest,
			Header:     base.Header{},
		}, liberrors.ErrServerCSeqMissing{}
	}

	sxID := getSessionID(req.Header)

	switch req.Method {
	case base.Options:
		res, err = r.OnOptions(req, connectionKey)
	case base.Announce:
		res, err = r.OnAnnounce(req, connectionKey)
	case base.Describe:
		res, err = r.OnDescribe(req, connectionKey)
	case base.Setup:
		res, err = r.OnSetup(req, connectionKey)
	case base.Play:
		if sxID != "" {
			res, err = r.OnPlay(req, connectionKey)
		} else {
			err = liberrors.ErrServerInvalidState{}
		}
	case base.Pause:
		if sxID != "" {
			res, err = r.OnPause(req, connectionKey)
		} else {
			err = liberrors.ErrServerInvalidState{}
		}
	case base.Record:
		if sxID != "" {
			res, err = r.OnRecord(req)
		} else {
			err = liberrors.ErrServerInvalidState{}
		}
	case base.Teardown:
		if sxID != "" {
			res, err = r.OnTeardown(req, connectionKey)
		} else {
			err = liberrors.ErrServerInvalidState{}
		}
	case base.GetParameter:
		if sxID != "" {
			res, err = r.OnGetParameter(req, connectionKey)
		} else {
			err = liberrors.ErrServerInvalidState{}
		}

	default:
		return &base.Response{StatusCode: base.StatusBadRequest}, liberrors.ErrServerUnhandledRequest{Request: req}
	}

	if err != nil {
		r.logError("error processing CP message")
		return nil, err
	} else {
		// reflect back the cSeq
		r.log("cseq = %v", cSeq)
		res.Header["CSeq"] = cSeq
		return res, nil
	}
}

func (r *RTSP) handleResponse(res *base.Response, connectionKey model.ConnectionKey) error {
	key := getRemoteIPv4Address(connectionKey.Remote)
	stubChannel, ok := r.stubChannels[key]

	if !ok {
		return errors.New("Can't load stub channel")
	}

	stubChannel.Response <- model.StubChannelResponse{
		nil,
		res,
	}
	stubChannel.ReceivedResponse = true
	r.log("received response %v", res)
	return nil
}

func getSessionID(header base.Header) string {
	if h, ok := header["Session"]; ok && len(h) == 1 {
		return h[0]
	}
	return ""
}
