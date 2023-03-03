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
	pb "github.com/media-streaming-mesh/msm-cp/api/v1alpha1/msm_cp"
	"github.com/media-streaming-mesh/msm-cp/internal/stub"
)

func (r *RTSP) handleRequest(req *base.Request, s *pb.Message) (*base.Response, error) {

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
		res, err = r.OnOptions(req, s)
	case base.Announce:
		res, err = r.OnAnnounce(req, s)
	case base.Describe:
		res, err = r.OnDescribe(req, s)
	case base.Setup:
		res, err = r.OnSetup(req, s)
	case base.Play:
		if sxID != "" {
			res, err = r.OnPlay(req, s)
		} else {
			err = liberrors.ErrServerInvalidState{}
		}
	case base.Pause:
		if sxID != "" {
			res, err = r.OnPause(req, s)
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
			res, err = r.OnTeardown(req, s)
		} else {
			err = liberrors.ErrServerInvalidState{}
		}
	case base.GetParameter:
		if sxID != "" {
			res, err = r.OnGetParameter(req, s)
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

func (r *RTSP) handleResponse(res *base.Response, s *pb.Message) error {
	key := getRemoteIPv4Address(s.Remote)
	stubConn, ok := stub.StubMap.Load(key)
	if !ok {
		return errors.New("Can't find stub connection")
	}

	stubConn.(*stub.StubConnection).DataCh <- res
	return nil
}

func getSessionID(header base.Header) string {
	if h, ok := header["Session"]; ok && len(h) == 1 {
		return h[0]
	}
	return ""
}
