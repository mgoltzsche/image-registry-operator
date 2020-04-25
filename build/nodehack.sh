#!/bin/sh

# Installs the CA cert on each node.
# If CRI-O is running it is reloaded afterwards.

[ $# -ne 0 ] || (echo "Usage: $0 setca|setdns|sleepinfinity..." >&2; false) || exit 1
[ "$HOST_PATH" ] || (echo HOST_PATH env var not specified >&2; false) || exit 1
[ -d "$HOST_PATH" ] || (echo HOST_PATH does not exist or is not a directory >&2; false) || exit 1

set -e${DEBUG}o pipefail


copyCaCert() {
	[ "$CERT_FILE" ] || (echo CERT_FILE env var not specified >&2; false) || exit 1
	[ "$CERT_NAME" ] || (echo CERT_NAME env var not specified >&2; false) || exit 1
	[ "$(cat $CERT_FILE)" ] || (echo $CERT_FILE is empty >&2; false) || exit 1
	CERT="$(cat $CERT_FILE)"
	CERT_HASH="$(echo "$CERT" | md5sum - | cut -d' ' -f1)"
	CERT_DIR=$1
	mkdir -p "$CERT_DIR"
	CERT_DEST=$CERT_DIR/registry-ca-${CERT_NAME}-${CERT_HASH}.crt
	if [ ! -f "$CERT_DEST" ]; then
		echo "$CERT" > $CERT_DIR/.tmp-registry-ca-cert-$CERT_NAME
		mv $CERT_DIR/.tmp-registry-ca-cert-$CERT_NAME $CERT_DEST
		# TODO: remove old certificates
		#for OLDCERT in $(ls $CERT_DIR | grep -E "^registry-ca-${CERT_NAME}-"'[0-9a-f]+\.crt' | grep -v $CERT_NAME); do
		#	rm -f $CERT_DIR/$OLDCERT
		#done
	fi
}

reloadcrio() {
	CRIO_PID="$(ps aux | grep -Em1 ' ([a-z/]+/)?crio( |$)' | grep -Eo '^\s*[0-9]+' | grep -Eo '[0-9]+')" || true
	if [ "$CRIO_PID" ]; then
		# Force CRI-O reload if crio process found to pick up new CA cert
		kill -1 "$CRIO_PID" || (echo ERROR: failed to reload crio >&2; false)
		echo CRI-O reloaded
	else
		echo 'WARNING: Could not find crio process.' >&2
		echo '  You may need to restart the container engine on each node' >&2
		echo '  manually to ensure the new CA certificate is registered.' >&2
	fi
}

setca() {
	copyCaCert $HOST_PATH/usr/local/share/ca-certificates
	rm -rf /etc/ssl/certs /usr/local/share/ca-certificates
	mkdir -p $HOST_PATH/etc/ssl $HOST_PATH/usr/local/share/ca-certificates
	ln -s $HOST_PATH/etc/ssl/certs /etc/ssl/certs
	ln -s $HOST_PATH/usr/local/share/ca-certificates /usr/local/share/ca-certificates
	update-ca-certificates
	if [ -d $HOST_PATH/etc/pki/ca-trust/source/anchors ]; then
		# required on RHEL/fedora/centos host:
		copyCaCert $HOST_PATH/etc/pki/ca-trust/source/anchors
		chroot $HOST_PATH /usr/bin/update-ca-trust
	fi
	echo CA certificate installed
}

setdns() {
	[ "$NAMESERVER" ] || (echo NAMESERVER env var not specified >&2; false) || exit 1
	if ! cat $HOST_PATH/etc/resolv.conf | grep -q "nameserver $NAMESERVER"; then
		RESOLVCONF="$(echo "nameserver $NAMESERVER" && cat $HOST_PATH/etc/resolv.conf)"
		echo "$RESOLVCONF" > $HOST_PATH/etc/.tmp.resolv.conf
		mv $HOST_PATH/etc/.tmp.resolv.conf $HOST_PATH/etc/resolv.conf ||
		# in case the host is a container: update resolv.conf insecurely
		echo "$RESOLVCONF" > $HOST_PATH/etc/resolv.conf
		rm -f $HOST_PATH/etc/.tmp.resolv.conf
	fi
	echo Nameserver $NAMESERVER configured
}

sleepinfinity() {
	sleep infinity
}


for TARGET in $@; do
	$TARGET
done
