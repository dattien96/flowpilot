# Until now we follow the detail step in CP-27 and get these result


GOOGLE_DRIVE_CLIENT_ID=94010348945-3ds34e26h3gqi2nr8g90smo7pbjojdmq.apps.googleusercontent.com
GOOGLE_DRIVE_CLIENT_SECRET=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
GOOGLE_PICKER_API_KEY=AIzaSyBcPDEZCENp0mwPO0uUwTlFD_PRGKfxZIQ
GOOGLE_DRIVE_REDIRECT_URI=http://127.0.0.1:4317/artifact-storage/google-drive/oauth/callback
GOOGLE_DRIVE_OAUTH_CREDENTIALS=C:\Users\dat.nguyen\.config\google-drive-mcp\gcp-oauth.keys.json
GOOGLE_DRIVE_MCP_TOKEN_PATH=C:\Users\dat.nguyen\.config\google-drive-mcp\tokens.json

New these values add in .env file

and the C:\Users\dat.nguyen\.config\google-drive-mcp\gcp-oauth.keys.json

in case this is a window project

But i see it is not good if we manually do it

I want we can create a new page in menu UI

# Bassically it show detail guide for 4 big steps

## step1 : create project in console
Just guide text is enough

## step2: Configure OAuth consent screen
Show guide steps by steps as we note in CP-27
Just guide text is enough

## Step3: Enable APIs
Just guide text is enough. follow Cp-27

## Step4 : Create OAuth clients for Runner callback - artifact feature
This step need code to support

###  Show guide for user follow these steps
- 1. In Google Cloud Console, go to `Google Auth Platform > Clients`.
2. Press `Create client`.
3. Choose application type `Web application`.
4. Name it something obvious, for example:
   - `FlowPilot Artifact Sync Local Runner`
5. Under redirect URIs, add:

```text
http://127.0.0.1:4317/artifact-storage/google-drive/oauth/callback
```

6. Save the client.
### Update - auto parts
But 2 final steps are
 7. Copy the generated `Client ID` and `Client secret`.
8. Put those values into:
   - `GOOGLE_DRIVE_CLIENT_ID`
   - `GOOGLE_DRIVE_CLIENT_SECRET`

I dont want to input manually
Let give me 2 text box uis
i can input my values got from google

The runner need open .env (created if not exist)
And fill in for me

Expectation: we have 3 var in env file like this
``

GOOGLE_DRIVE_CLIENT_ID=94010348945-3ds34e26h3gqi2nr8g90smo7pbjojdmq.apps.googleusercontent.com
GOOGLE_DRIVE_CLIENT_SECRET=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
GOOGLE_DRIVE_REDIRECT_URI=http://127.0.0.1:4317/artifact-storage/google-drive/oauth/callback
``

## Step5: Create OAuth clients DESKTOP for 3rd MCP lib - MCP feature
###  Show guide for user follow these steps

1. In Google Cloud Console, go to `Google Auth Platform > Clients`.
2. Press `Create client`.
3. Choose application type `Desktop app`.
4. Name it something obvious, for example:
   - `FlowPilot Google Drive MCP`
5. Save the client.
6. Download the OAuth client JSON file from Google Cloud.
### Update - auto parts
But after download the file i expect we have 1 component UI
allow me drag me file to it

Then the web/runner copy that file to 
```text
~/.config/google-drive-mcp/gcp-oauth.keys.json
```

pls note for Win/Mac/Linux

## Step6: Create API key for Picker
###  Show guide for user follow these steps
1. In Google Cloud Console, go to `APIs & Services > Credentials`.
2. Press `Create credentials`.
3. Choose `API key`.
4. Fill the form fields like this:

   - `Name`
     Use a clear name, for example:

```text
FlowPilot Artifact Picker Key
```

   - `APIs that can be accessed using this key` or `Select API restrictions`
     Select `Google Picker API`.

     If `Google Picker API` is not available in the list, go back to `APIs & Services > Library`, enable `Google Picker API`, then return to the key form.

   - `Authenticate API calls through a service account`
     Leave this unchecked.

     Reason:
     this Picker key is for browser-side app identification, not for service-account-based server auth.

   - `Application restrictions`
     For the very first local test, you can leave this as `None` so you can confirm the Picker flow works.

     After that first successful test, tighten it.

     Recommended restriction for the current runner-hosted Picker page:
     choose `Websites`.

     Add website/referrer entries that match the runner page origin. For the default local runner URL, use:

```text
http://127.0.0.1:4317/*
```

     If you also access the runner through `localhost`, add:

```text
http://localhost:4317/*
```

     If you change `FLOWPILOT_RUNNER_URL`, the website restriction values must change to match that origin.

1. Create the key.
2. Copy the created key.

### Update - auto parts
Ui for input that key

Runer Check env file and add to env 
GOOGLE_PICKER_API_KEY=... for me