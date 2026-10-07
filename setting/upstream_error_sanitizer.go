package setting

import "github.com/QuantumNous/new-api/common"

// SanitizeUpstreamErrorEnabled hides upstream error text from downstream
// clients. Gateways forward failures verbatim today, so an upstream body —
// which routinely carries provider account identifiers, balance or quota
// figures, internal hostnames and URLs, request IDs of another system, and
// sometimes fragments of the prompt — reaches whoever called the API.
//
// While enabled, the text served to non-admin API callers is replaced by a
// fixed sentence. What is preserved in the response: the HTTP status code, the
// protocol error type, and the error code, so clients can still branch on
// `insufficient_quota`, `rate_limit_exceeded`, `content_policy_violation` and
// friends. Internally the verbatim upstream error is kept everywhere it
// matters: channel auto-disable keyword/status-code matching, retry
// classification, and the error logs all compare against the real error, and
// administrators and root also receive the verbatim error in their own API
// responses.
//
// Errors this service authored itself (invalid request, insufficient quota of
// our own wallet, channel selection failures, rate limiting) keep their
// message, because the client cannot act on those failures otherwise.
//
// Bootstrap default comes from SANITIZE_UPSTREAM_ERROR; the root setting of the
// same name overrides it at runtime. On by default: forwarding upstream error
// bodies verbatim is a disclosure defect, not a feature, and the sanitized
// response still carries the HTTP status, protocol error type and error code so
// clients keep their error handling. Deployments that depend on the verbatim
// text can set SANITIZE_UPSTREAM_ERROR=false or flip the root setting.
var SanitizeUpstreamErrorEnabled = true

func init() {
	SanitizeUpstreamErrorEnabled = common.GetEnvOrDefaultBool("SANITIZE_UPSTREAM_ERROR", true)
}
