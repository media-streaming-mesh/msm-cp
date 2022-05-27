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
)

func (r *RTSP) handleRequest(req *base.Request, s *pb.Message) (*base.Response, error) {

	if cSeq, ok := req.Header["CSeq"]; !ok || len(cSeq) != 1 {
		r.logger.Error("CSeq missing")

		return &base.Response{
			StatusCode: base.StatusBadRequest,
			Header:     base.Header{},
		}, liberrors.ErrServerCSeqMissing{}
	}

	sxID := getSessionID(req.Header)

	switch req.Method {
	case base.Options:
		res, err := r.OnOptions(req, s)
		if err != nil {
			return nil, err
		}

		return res, err
	case base.Announce:
		res, err := r.OnAnnounce(req, s)
		if err != nil {
			return nil, err
		}

		return res, nil
	case base.Describe:
		res, err := r.OnDescribe(req, s)
		if err != nil {
			return nil, err
		}

		return res, err
	case base.Setup:
		res, err := r.OnSetup(req, s)
		if err != nil {
			return nil, err
		}

		return res, err
	case base.Play:
		if sxID != "" {
			res, err := r.OnPlay(req, s)
			if err != nil {
				return nil, err
			}

			return res, err
		}
	case base.Record:
		if sxID != "" {
			res, err := r.OnRecord(req)
			if err != nil {
				return nil, err
			}

			return res, err
		}

	}
	return &base.Response{
		StatusCode: base.StatusBadRequest,
	}, liberrors.ErrServerUnhandledRequest{Request: req}

}

func (r *RTSP) handleResponse(res *base.Response, s *pb.Message) error {

	key := getRemoteIPv4Address(s.Remote)
	stubConn, ok := r.stubConn.Load(key)
	if !ok {
		r.logger.Errorf("This is just bad")
		return errors.New("shit2")
	}

	stubConn.(*StubConnection).dataCh <- res

	return nil
}

func getSessionID(header base.Header) string {
	if h, ok := header["Session"]; ok && len(h) == 1 {
		return h[0]
	}
	return ""
}
