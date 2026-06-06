until now we have CP-28 task, some parts i want to change

1. Page /settings/google-drive-setup
When the status of Artifact sync is configured in summary card
That mean we setup enough data of google console for the artifact sync feature
-> In this state, show 1 button to navigate to /artifacts page with tab Shared Cloud Storage Setting


1. page /artifacts tab: Shared Cloud Storage Setting

Currently we show Active provider for this project

Supabase Storage

supabase
By default

But i want to have 2 sections for
1 Select Supabase
2 Select Google Drive
2.1 Check do we have setup google console for data this feature need ?
If not -> show 1 button nav to /settings/google-drive-setup
2.2 If we have setup google console for data this feature need -> show Connect button
2.3 User can continue to connect google drive
2.4 User click save/select to explicity choose driver

user can still re-select supabase