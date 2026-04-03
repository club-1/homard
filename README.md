🦞 homard
=========

[![build][build-svg]][build-url] [![coverage][cover-svg]][cover-url]

Milter to add SMTP AUTH Authentication-Results field to self-sent mails.

Mails from authenticated clients will usually be processed differently by
milters such as OpenDKIM and OpenDMARC. OpenDKIM will sign the mail instead
of verifying it, and OpenDMARC will simply ignore it. As a result, mails
sent by authenticated clients will not have any `Authentication-Results`
fields in their header. For outgoing mails, they will be verified by their
recipient, but for local mails, they will stay as is.

Having a mail without any `Authentication-Results` field can confuse MUAs
such as FairEmail, that use it to display the authentication status of a
mail.

To fix this issue, homard adds a SMTP AUTH `Authentication-Results` field
as described by [RFC7601 § 2.7.4] to the header of mails sent by authenticated
clients. This is usually enough for MUAs to consider the mail as fully
authenticated (as it is)

[RFC7601 § 2.7.4]: https://datatracker.ietf.org/doc/html/rfc7601#section-2.7.4

<img src="https://static.club1.fr/nicolas/projects/homard/screenshot-fairemail.png" alt="fairemail screenshot" width="400px">

_In this screenshot, the second mail is sent with homard whereas the first is not._

Configuration with Postfix on Debian
------------------------------------

Postfix is run in a chroot in Debian, so it is needed to adapt the default
configuration. Create a new directory for the UNIX socket in Postfix's chroot
with the correct permissions, for example with systemd-tmpfiles:

```ini
#/etc/tmpfiles.d/homard.conf
#Type  Path                          Mode  User       Group    Age  Argument
d      /var/spool/postfix/homard     0750  homard     postfix  -    -
```

Then:

    sudo systemd-tmpfiles --create

Set the ListenURI in homard's config file:

```toml
# /etc/homard.conf
ListenURI = "unix:///var/spool/postfix/homard/homard.sock"
```

Add homard to postfix's milters in `/etc/postfix/main.cf`:

```diff
 smtpd_milters =
   local:opendkim/opendkim.sock
   local:opendmarc/opendmarc.sock
+  local:homard/homard.sock

 non_smtpd_milters =
   local:opendkim/opendkim.sock
+  local:homard/homard.sock
```

Add postfix to the homard group:

    sudo adduser postfix homard

And finally restart both services:

    sudo systemctl restart postfix homard

About the name
--------------

The name kind of means HOMe Authentication-Results Daemon, but it was mostly
chosen because it means "lobster" in French and ends with "ard".


[build-svg]: https://github.com/club-1/homard/actions/workflows/build.yml/badge.svg
[build-url]: https://github.com/club-1/homard/actions/workflows/build.yml
[cover-svg]: https://github.com/club-1/homard/wiki/coverage.svg
[cover-url]: https://raw.githack.com/wiki/club-1/homard/coverage.html
