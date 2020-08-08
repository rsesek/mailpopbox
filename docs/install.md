# Mailpopbox Installation Guide

This guide covers how to install mailpopbox, set up auto-renewing TLS certificates, and setting up
Gmail to POP messages off the server.

## Server

Mailpopbox can be deployed on a variety of operating system environments, but this guide is for a
Linux server with:

- Root access and the ability to run long-lived processes (i.e. not shared hosting)
- The **iptables** package

I recommend [Digital Ocean droplets](https://www.digitalocean.com/products/droplets/), which cost
$5/month. This guide does not cover how to properly secure a Linux server, such as SSH configuration
and firewalls; please use other guides to do that if this is a new server instance.

## Install Mailpopbox

These commands assume you are running as root; if not, precede the commands with `sudo`.

1. Download the latest mailpopbox release and copy the binary to `/usr/local/bin`

2. Set ownership and permissions:
    - `chown root:wheel /usr/local/bin/mailpopbox`
    - `chmod 755 /usr/local/bin/mailpopbox`

3. Create a new user that the server will run under: `useradd mailpopbox --shell /sbin/nologin`

4. Create some directories with the correct permissions:
    - `cd /home/mailpopbox`
    - `mkdir -p maildrop/yourdomain.com cert www/.well-known/acme-challenge`
    - `chown -R mailpopbox:mailpopbox maildrop cert www`

5. Create a file at `/home/mailpopbox/config.json`:

    ```
    {
        "SMTPPort": 9025,
        "POP3Port": 9995,
        "Hostname": "mx.yourdomain.com",
        "Servers": [
            {
                "Domain": "yourdomain.com",
                "MailboxPassword": "yourpassword",
                "TLSKeyPath": "/home/mailpopbox/cert/certificates/mx.yourdomain.com.key",
                "TLSCertPath": "/home/mailpopbox/cert/certificates/mx.yourdomain.com.crt",
                "MaildropPath": "/home/mailpopbox/maildrop/yourdomain.com"
            }
        ]
    }
    ```

    - The SMTP and POP3 ports are not the values that will be exposed to the Internet, as those are
        reserved ports that require root access to bind. Instead, mailpopbox binds these
        unprivileged ports and *iptables* will be used to route Internet traffic to the server. This
        is handled by the included systemd unit.
    - The `Hostname` is the MX server hostname. Multiple catch-all domains can be configured on a
        single server, but they will all share this MX hostname in e.g. the SMTP HELO.
    - The `Domain` is the domain name for which `*@yourdomain.com` will be set up.
    - The `MailboxPassword` is the password for the `mailbox@yourdomain.com` account, used to
        authenticate POP3 and outbound SMTP connections. Choose a strong (preferably random)
        password!
    - The `TLSKeyPath` and `TLSCertPath` are used to find the TLS certificate, which will be
        configured below.
    - The `MaildropPath` is where delivered messages are stored until they are POP'd off the
        server.

## Configure DNS

1. Add a DNS A record to `yourdomain.com`, configuring the subdomain `mx.yourdomain.com` to point to
the public IP address of the server running mailpopbox.

2. Set or change the DNS MX record of `yourdomain.com` to point to `mx.yourdomain.com`.

Changes to DNS can take up between 1 and 24 hours to propagate. The DNS entries need to be
configured in order to continue installation.

## Setup Automatic TLS Certificates

This guide will assume that your instance of mailpopbox is running on a system that also has a
web server running. The web server will be used to host [ACME
certificate](https://en.wikipedia.org/wiki/Automated_Certificate_Management_Environment) challenges.
I recommend the [lego](https://github.com/go-acme/lego) tool for certificate management.

> If you already have a mechanism to get certificates, you can use that and just adjust the paths in
> `config.json`. Also be sure to configure a hook for your auto-renew mechanism to restart mailpopbox
> when a new certificate is installed.

1. Install [lego](https://github.com/go-acme/lego) on the server.

2. Configure your web server to serve the content under the mailpopbox home directory. With an
apache2 configuration, this can be done by editing `/etc/httpd/conf.d/mx.yourdomain.com` with this
content:

    ```
    <VirtualHost *:80>
        ServerName mx.yourdomain.com

        ErrorLog logs/yourdomain_error_log
        CustomLog logs/yourdomain_access_log combined

        DocumentRoot /home/mailpopbox/www
    </VirtualHost>

    <Directory /home/mailpopbox/www>
        Require all granted
    </Directory>
    ```

    You may also need to adjust the permissions so the web server can serve the files:
    - `chmod o+x /home/mailpopbox`
    - `chmod o+rx /home/mailpopbox/www`

3. Reload the web server `systemctl reload httpd.service`

4. Customize this command and register for the initial account and certificate:

        sudo -u mailpopbox /usr/local/bin/lego --email email@domain.com --domains mx.yourdomain.com --http --http.webroot /home/mailpopbox/www --path /home/mailpopbox/cert run

  This will register *email@domain.com* with [LetsEncrypt](https://letsencrypt.org) to get a
  certificate for *mx.yourdomain.com* by putting an authentication challenge file in the
  `/home/mailpopbox/www` directory, with the resulting certificate files in `/home/mailpopbox/cert`.


5. Let mailpopbox restart itself via systemd by editing the sudoers file with `visudo`, and add this
line:

        mailpopbox ALL=(root) NOPASSWD: /usr/bin/systemctl restart mailpopbox.service

6. Set up a cron job to automatically renew the certificate using `sudo crontab -u mailpopbox -e`
and specifying this command (which is nearly the same as the `run` above, except it uses `renew` and
a hook). Customize this command:

        0 22 * * * /usr/local/bin/lego --email email@domain.com --domains mx.yourdomain.com --http --http.webroot /home/mailpopbox/www --path /home/mailpopbox/cert renew --renew-hook "sudo /usr/bin/systemctl restart mailpopbox.service"

## Starting Mailpopbox

1. Copy the `mailpopbox.service` systemd unit file from the release archive to
`/usr/local/lib/systemd/system`

> Note that the systemd unit file uses `/sbin/iptables` to forward traffic from ports 25 and 995 to
> the ports specified in the `config.json` file. It also specifies the path to the `config.json`
> file.

2. Enable the systemd unit with `sudo systemctl enable mailpopbox.service`

3. Start mailpopbox with `sudo systemctl start mailpopbox.service`

4. Verify that the server has started with `sudo journalctl -u mailpopbox`

5. Test the connection to the server from your local machine: `openssl s_client -connect
mx.yourdomain.com:995`. You should see your certificate printed by `openssl` and then a line that
says `+OK POP3 (mailpopbox) server mx.yourdomain.com`.

## Configuring Your Email Client

Now that mailpopbox is running and DNS is configured, it is time to set your mail client up to
connect to it. We will set up both a POP3 account to download delivered mail, and a SMTP account to
enable replying. This guide is for Gmail, but the configuration parameters are the same regardless
of client.

1. Go to **Gmail Settings** > **Accounts and Import**

2. Under **Check mail from other accounts:**, click **Add a mail account**

3. Specify `mailbox@yourdomain.com` as the email address and click **Next**

4. Choose **Import emails from my other account (POP3)** and click **Next**

5. Specify the following configuration and add the account:
    - **Username:** `mailbox@yourdomain.com`
    - **Password:** The password you specified in `config.json`
    - **POP Server:** `mx.yourdomain.com`
    - **Port:** `995`
    - Check **Always use a secure connection (SSL) when retrieving mail**
    - Optionally, apply a label to all the messages sent to this domain

Gmail will now fetch messages delivered to the server. You can send a test message to
`install-test@yourdomain.com` and it should be delivered within a few minutes. Note that Gmail
detects when a newly delivered message is the same as one in the *Sent* folder and it discards it;
use a separate email account for this test. Gmail also fetches mail from the POP account
periodically via polling, so message delivery can seem slower than normal.

1. Go to **Gmail Settings** > **Accounts and Import**

2. Under **Send mail as:**, click **Add another email address**

3. Specify the following and click **Next**:
    - **Name:** Whatever you prefer
    - **Email:** `mailbox@yourdomain.com`
    - Check **Treat as an alias**

4. Specify the following and add the account:
    - **SMTP Server:** `mx.yourdomain.com`
    - **Port:** `25`
    - **Username:** `mailbox@yourdomain.com`
    - **Password:** The password you specified in `config.json`
    - Check **Secured connection using TLS**

Gmail will now let you send email as `mailbox@yourdomain.com`. But if you are replying to a message
sent to `random@yourdomain.com`, you do not want the recipient to see the "mailbox" username in your
reply. If you append `[sendas:random]` to the Subject line of the message, the SMTP server will
change the From address to `random@yourdomain.com` and remove the special tag from the Subject line.
