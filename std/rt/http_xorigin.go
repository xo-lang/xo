package rt

import (
	nethttp "net/http"
	"net/url"
)

// Http_CrossOriginErr's display is its Go text.
func (e *Http_CrossOriginErr) M_display(_ *Task) string { return e.F_msg }

// Http_cross_origin is `http.cross_origin(trusted, bypass)`: the origins
// checked as Go's CrossOriginProtection.AddTrustedOrigin checks them.
func Http_cross_origin(tk *Task, trusted, bypass List[string]) Result[Http_CrossOrigin, Http_CrossOriginErr] {
	c := nethttp.NewCrossOriginProtection()
	for _, o := range trusted.s {
		if err := c.AddTrustedOrigin(o); err != nil {
			return Err[Http_CrossOrigin](Http_CrossOriginErr{F_msg: err.Error()})
		}
	}
	return Ok[Http_CrossOrigin, Http_CrossOriginErr](Http_CrossOrigin{F_trusted: SetOf(trusted.s...), F_bypass: bypass})
}

// protection is c as Go's CrossOriginProtection.
func (c *Http_CrossOrigin) protection(tk *Task) *nethttp.CrossOriginProtection {
	p := nethttp.NewCrossOriginProtection()
	for _, o := range Set_to_list(tk, c.F_trusted).s {
		_ = p.AddTrustedOrigin(o)
	}
	for _, b := range c.F_bypass.s {
		p.AddInsecureBypassPattern(b)
	}
	return p
}

// M_check is `c.check(req)`: CrossOriginProtection.Check on the request's
// method, Host, path and fields.
func (c *Http_CrossOrigin) M_check(tk *Task, req Http_Request) Result[Unit, Http_CrossOriginErr] {
	hr := &nethttp.Request{Method: req.F_method, Host: req.XHost, URL: &url.URL{Path: req.F_path}, Header: nethttp.Header{}}
	for _, k := range []string{"Sec-Fetch-Site", "Origin"} {
		if v := headerGet(req.F_headers, k); v.Some {
			hr.Header.Set(k, v.V)
		}
	}
	if err := c.protection(tk).Check(hr); err != nil {
		return Err[Unit](Http_CrossOriginErr{F_msg: err.Error()})
	}
	return Ok[Unit, Http_CrossOriginErr](Unit{})
}

// M_middleware is `c.middleware()`: 403 with check's text as http.Error
// writes it.
func (c *Http_CrossOrigin) M_middleware(tk *Task) func(*Task, Http_Request, func(*Task, Http_Request) Result[Http_Response, Never]) Result[Http_Response, Never] {
	cc := *c
	return func(tk *Task, req Http_Request, next func(*Task, Http_Request) Result[Http_Response, Never]) Result[Http_Response, Never] {
		if r := cc.M_check(tk, req); !r.Ok {
			h := mapCap[string, string](2)
			Map_put(tk, &h, "Content-Type", "text/plain; charset=utf-8")
			Map_put(tk, &h, "X-Content-Type-Options", "nosniff")
			return Ok[Http_Response, Never](Http_Response{F_status: 403, F_headers: h, F_body: Bytes(r.E.F_msg + "\n")})
		}
		return next(tk, req)
	}
}
