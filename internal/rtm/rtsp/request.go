/*
 * Copyright (c) 2022 Cisco and/or its affiliates.
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
	"bufio"
	"fmt"
	"net/url"
	"strings"
)

const (
	_MAX_METHOD_LENGTH   = 128
	_MAX_PATH_LENGTH     = 1024
	_MAX_PROTOCOL_LENGTH = 128
)

// Method is a RTSP request method.
type Method string

const (
	ANNOUNCE      Method = "ANNOUNCE"
	DESCRIBE      Method = "DESCRIBE"
	GET_PARAMETER Method = "GET_PARAMETER"
	OPTIONS       Method = "OPTIONS"
	PAUSE         Method = "PAUSE"
	PLAY          Method = "PLAY"
	PLAY_NOTIFY   Method = "PLAY_NOTIFY"
	RECORD        Method = "RECORD"
	REDIRECT      Method = "REDIRECT"
	SETUP         Method = "SETUP"
	SET_PARAMETER Method = "SET_PARAMETER"
	TEARDOWN      Method = "TEARDOWN"
)

// Request is a RTSP request.
type Request struct {
	// request method
	Method Method

	// request url
	Url *url.URL

	// map of header values
	Header Header

	// optional content
	Content []byte
}

func (r *RTSP) handleRequest(req *Request) *Response {
	r.logger.Debugf("Method %s, URL %s", string(req.Method), req.Url.String())

	cseq, ok := req.Header["CSeq"]
	if !ok || len(cseq) != 1 {
		return r.writeResError(req, StatusBadRequest, fmt.Errorf("cseq missing"))
	}

	path := func() string {
		ret := req.Url.Path

		// remove leading slash
		if len(ret) > 0 {
			ret = ret[1:]
		}

		// strip any subpath
		if n := strings.Index(ret, "/"); n >= 0 {
			ret = ret[:n]
		}

		return ret
	}()

	r.logger.Debugf("path: %s", path)

	switch req.Method {
	case OPTIONS:
		// do not check state, since OPTIONS can be requested
		// in any state

		res := &Response{
			StatusCode: StatusOK,
			Header: Header{
				"CSeq": []string{cseq[0]},
				"Public": []string{strings.Join([]string{
					string(DESCRIBE),
					string(SETUP),
					string(PLAY),
					string(PAUSE),
					string(TEARDOWN),
				}, ", ")},
			},
		}
		r.logger.Debugf("response: %+v", res)
		return res

	case DESCRIBE:

		res := &Response{
			StatusCode: StatusOK,
			Header: Header{
				"CSeq":         []string{cseq[0]},
				"Content-Base": []string{req.Url.String() + "/"},
				"Content-Type": []string{"application/sdp"},
			},
			// todo - content func
			Content: []byte{},
		}
		r.logger.Debugf("response: %+v", res)
		return res

	case SETUP:
		tsRaw, ok := req.Header["Transport"]
		if !ok || len(tsRaw) != 1 {
			return r.writeResError(req, StatusBadRequest, fmt.Errorf("transport header missing"))
		}

		th := ReadHeaderTransport(tsRaw[0])
		if _, ok := th["multicast"]; ok {
			return r.writeResError(req, StatusBadRequest, fmt.Errorf("multicast is not supported"))
		}

		res := &Response{
			StatusCode: StatusOK,
			Header: Header{
				"CSeq": []string{cseq[0]},
				"Transport": []string{strings.Join([]string{
					"RTP/AVP/UDP",
					"unicast",
					fmt.Sprintf("client_port=%d-%d", 4132, 4324),
					fmt.Sprintf("server_port=%d-%d", 5342, 5435),
				}, ";")},
				"Session": []string{"12345678"},
			},
		}
		r.logger.Debugf("response: %+v", res)
		return res

	case PLAY:

		res := &Response{
			StatusCode: StatusOK,
			Header: Header{
				"CSeq":    []string{cseq[0]},
				"Session": []string{"12345678"},
			},
		}
		r.logger.Debugf("response: %+v", res)
		return res

	case PAUSE:

		res := &Response{
			StatusCode: StatusOK,
			Header: Header{
				"CSeq":    []string{cseq[0]},
				"Session": []string{"12345678"},
			},
		}
		r.logger.Debugf("response: %+v", res)
		return res

	case TEARDOWN:

		res := &Response{
			StatusCode: StatusOK,
			Header: Header{
				"CSeq": []string{cseq[0]},
			},
		}
		r.logger.Debugf("response: %+v", res)
		return res

	default:
		r.writeResError(req, StatusBadRequest, fmt.Errorf("unhandled method '%s'", req.Method))
	}

	return nil
}

func readRequest(rb *bufio.Reader) (*Request, error) {
	req := &Request{}

	byts, err := readBytesLimited(rb, ' ', _MAX_METHOD_LENGTH)
	if err != nil {
		return nil, err
	}
	req.Method = Method(byts[:len(byts)-1])

	if req.Method == "" {
		return nil, fmt.Errorf("empty method")
	}

	byts, err = readBytesLimited(rb, ' ', _MAX_PATH_LENGTH)
	if err != nil {
		return nil, err
	}
	rawUrl := string(byts[:len(byts)-1])

	if rawUrl == "" {
		return nil, fmt.Errorf("empty url")
	}

	ur, err := url.Parse(rawUrl)
	if err != nil {
		return nil, fmt.Errorf("unable to parse url '%s'", rawUrl)
	}
	req.Url = ur

	if req.Url.Scheme != "rtsp" {
		return nil, fmt.Errorf("invalid url scheme '%s'", req.Url.Scheme)
	}

	byts, err = readBytesLimited(rb, '\r', _MAX_PROTOCOL_LENGTH)
	if err != nil {
		return nil, err
	}
	proto := string(byts[:len(byts)-1])

	if proto != _RTSP_PROTO {
		return nil, fmt.Errorf("expected '%s', got '%s'", _RTSP_PROTO, proto)
	}

	err = readByteEqual(rb, '\n')
	if err != nil {
		return nil, err
	}

	req.Header, err = headerRead(rb)
	if err != nil {
		return nil, err
	}

	req.Content, err = readContent(rb, req.Header)
	if err != nil {
		return nil, err
	}

	return req, nil
}

func (req *Request) write(bw *bufio.Writer) error {
	_, err := bw.Write([]byte(string(req.Method) + " " + req.Url.String() + " " + _RTSP_PROTO + "\r\n"))
	if err != nil {
		return err
	}

	err = req.Header.write(bw)
	if err != nil {
		return err
	}

	err = writeContent(bw, req.Content)
	if err != nil {
		return err
	}

	return bw.Flush()
}

func (r *RTSP) writeResError(req *Request, code StatusCode, err error) *Response {
	r.logger.Debugf("WRITE RES ERR: %s", err)

	header := Header{}
	if cseq, ok := req.Header["CSeq"]; ok && len(cseq) == 1 {
		header["CSeq"] = []string{cseq[0]}
	}

	return &Response{
		StatusCode: code,
		Header:     header,
	}
}
