// Package sign will implement the v1 webhook signature scheme
// (HMAC-SHA256 over "{timestamp}.{raw_body}", header
// X-Courier-Signature: v1=<hex>) and its verifier.
//
// Nothing is implemented yet. When it is, every exported function must be
// covered by golden table tests and a fuzz test (SRS §8.9).
package sign
