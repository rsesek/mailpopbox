# Mailpopbox-Router Installation Guide

This guide covers how to install mailpopbox-router to move messages between the mailpopbox POP3
server (or any POP3 server) and Gmail.

Mailpopbox-router does not require any public inbound network exposure, only outbound access to
your mail server and Gmail. It does require an OAuth client redirect URI, which can be local to
your private network. Using TLS for this is optional but recommended; setup is outside of the scope
of this guide.

## Google Cloud Project Configuration

In order to deliver messages to Gmail, you need to configure a Google Cloud project and OAuth
client.

1. Create a new GCP project on [](https://console.cloud.google.com).

2. Under **APIs & Services**, enable the **Gmail API**.

3. Under **APIs & Services**, go to the **OAuth Consent Screen** and fill in the required details.

4. You can use the Gmail API without getting Google security approval by keeping it in the "Testing"
   phase and using "Test users" under **Google Auth Platform** > **Audience**. Add all the Google
   account addresses (emails) that you will be using mailpopbox-router with to the **Test users**
   section. Note you are limited to 100 users for the lifetime of the project.

5. Go to **Google Auth Platform** > **Data Access**. Click **Add or remove scopes** and add the
   scope `https://www.googleapis.com/auth/gmail.insert`, which may be abbreviated to
   `.../auth/gmail.insert`.

6. Finally go to **Google Auth Platform** > **Clients**. Create a new client of type **Web
   application**. For the **Authorized redirect URLs** you will need to specify the full URL
   (including scheme and any nonstandard port) that the OAuth server will redirect the client to.
   This should point to the host that runs mailpopbox-router.

7. Download the client credentials JSON file and store it in a secure location.

> [!note]
>
> Unfortunately, if your Google account is enrolled in the Advanced Protection Program, you will not
> be able to use an OAuth app with a restricted scope. That includes the setup described above, even
> for the very limited `gmail.insert` scope. You have to disable Advanced Protection in order to use
> this setup.

## Install Mailpopbox-Router

The easiest way to run mailpopbox-router is via a container. One port must be published for the
OAuth webserver callback, and a volume must be mounted to provide the configuration file and OAuth
token storage.

A sample config file looks like this, which will move messages from a POP3 account to a Gmail
account. The POP3 server will be polled every 90 seconds, and it listens on container port 80.

```json
{
  "Monitor": [
    {
      "Source": {
        "Type": "pop3",
        "ServerAddr": "mx.myserver.example:995",
        "UseTLS": true,
        "Email": "mailbox@myserver.example",
        "Password": "the-password-to-the-account"
      },
      "Destination": {
        "Type": "gmail",
        "Email": "my-gmail-account@gmail.com"
      },
      "PollIntervalSeconds": 90
    }
  ],
  "OAuthServer": {
    "RedirectURL": "http://mailpopbox-router.mylocal.network",
    "ListenAddr": ":80",
    "CredentialsPath": "/var/mailpopbox/google_creds.json",
    "TokenStore": "/var/mailpopbox/router_oauth_tokens.json"
  }
}
```

Quick start:

```shell
$ mkdir mailpopbox_router_data
$ vim mailpopbox_router_data/router.json
# paste the sample config above and modify appropriately
$ cp ~/Downloads/google_oauth_secrets.json mailpopbox_router_data/google_creds.json
$ docker run -v ./mailpopbox_router_data:/var/mailpopbox -p 8080:80 ghcr.io/rsesek/mailpopbox/mailpopbox-router:latest
```

A few notes on the container approach:

- The container for mailpopbox-router expects the configuration file at
  `/var/mailpopbox/router.json`
- The `OAuthServer.ListenAddr` should listen on all interfaces in the container
- To serve the OAuth server with HTTPS, use a reverse proxy that performs TLS termination and
  forwards to the container port.
