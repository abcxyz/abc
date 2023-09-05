# REST server deployments template

This directory contains the Dockerfile for the REST server template (needed for Cloud Run deployment).

    $ abc templates render github.com/abcxyz/abc.git//t/rest_server/deployments
 
 In the rare case where you want to install to a custom directory, optionally use `--input="subfolder=custom/location"` to specify render location relative to `--dest`.

    $ abc templates render --input="subfolder=custom/location" github.com/abcxyz/abc.git//t/rest_server/deployments
