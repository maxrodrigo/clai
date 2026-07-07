/*
 * Apple SSL/TLS bug (CVE-2014-1266)
 * This code broke certificate validation on all iOS and macOS devices.
 * https://dwheeler.com/essays/apple-goto-fail.html
 */

static OSStatus
SSLVerifySignedServerKeyExchange(SSLContext *ctx, bool isRsa,
                                  SSLBuffer signedParams,
                                  uint8_t *signature,
                                  UInt16 signatureLen)
{
    OSStatus err;

    if ((err = ReadyHash(&SSLHashSHA1, &hashCtx)) != 0)
        goto fail;
    if ((err = SSLHashSHA1.update(&hashCtx, &clientRandom)) != 0)
        goto fail;
    if ((err = SSLHashSHA1.update(&hashCtx, &serverRandom)) != 0)
        goto fail;
    if ((err = SSLHashSHA1.update(&hashCtx, &signedParams)) != 0)
        goto fail;
        goto fail;
    if ((err = SSLHashSHA1.final(&hashCtx, &hashOut)) != 0)
        goto fail;

    err = sslRawVerify(ctx, ctx->peerPubKey, dataToSign, dataToSignLen,
                       signature, signatureLen);

fail:
    SSLFreeBuffer(&signedHashes);
    SSLFreeBuffer(&hashCtx);
    return err;
}
