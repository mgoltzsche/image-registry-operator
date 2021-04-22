#!/bin/sh

set -eu

test "$VERSION"
test "$KUSTOMIZATION_DIR"

RELEASE_NAME="Test Release $VERSION"
TMPDIR=`mktemp -d`
STATUS=0
(
	cp -rf . $TMPDIR &&
	cd $TMPDIR &&
	kustomize cfg set $KUSTOMIZATION_DIR registry-manager-image mgoltzsche/image-registry-operator:${VERSION} &&
	kustomize cfg set $KUSTOMIZATION_DIR registry-auth-image mgoltzsche/image-registry-operator:${VERSION}-auth &&
	kustomize cfg set $KUSTOMIZATION_DIR registry-nginx-image mgoltzsche/image-registry-operator:${VERSION}-nginx &&
	git add $KUSTOMIZATION_DIR &&
	git commit -m"$RELEASE_NAME" &&
	git tag -a "v$VERSION" -m"$RELEASE_NAME" &&
	git push --tags
) || STATUS=1
rm -rf $TMPDIR
exit $STATUS
