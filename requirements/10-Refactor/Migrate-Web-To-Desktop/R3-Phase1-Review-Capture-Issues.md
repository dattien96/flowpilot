This doc keep all issues i found after test the desktop app
after PHASE 1 of R3-Migrate-Web-To-Desktop.md + R3-Phase1-Checklist.md was implemented

1. (DONE) Move the tab supabase to the bottom, above Runner tab
2. (DONE) Runner tab must have 1 circle idicator right next to the text Runner in the menu
3. (Done) Missing Connect new acc for each provider in Provider setting part. See /settings/accounts in web ver to know how to do it
4. (DONE) Page Project: /projects in web
  1. (DONE) Should NOT Show the create form -> use 1 Create cirle button -> press nav to new route in this menu tab for creating new project
  2. (DONE) The main page of this tab needs has 2 sides: left side shows a list of project, the righ side is the detail of that project
  3. (DONE) In detail page, each item should be a collapsible list, Default is exapand 1st item
  4. (DONE) Change item name:Integrations to MCP
  5. (DONE) Missing Artifact item to show a list of artifact gen in this project
  6. (DONE) Each item: Teams,Run History, MCP, Artifact: must 1 button to nav to that page. for example i can see the team list now. but i want to create more or see detail members before link to this project
  7. (DONE) Missing the button to DEl current project. When del, show 1 form that request User input exactly "the text : delete to enable the DEL button in that modal form
5. (DONE) Workflows page: refer: /workflows + /workflow-steps in web
  1. (DONE) Update the menu name from Workflows -> Workflows/Steps
  2. (DONE) Missing button del workflow + step
  3. (DONE) Pressing Create workflow auto create new empty workflow is not correct
    1. (DONE) The form for all page like this is: main page show current data: left side is list. right side is detail. 1 del + create button. Create button shows another page in this tab for creating form. Detail page for current item can show in edit mode. But only enable/highligh the SAVE button when we change the data
    2. (DONE) Update for both workflow and step
  4. (DONE) Workflow Detail missed: Reasoning effort + YOLO mode setting
  5. (DONE) Step detail missed: Required MCPs + Required skills + Prompt base + Subagent + Reasoning + Input artifact definitions + Ouput artifact definitions
6. (DONE) Page Teams: ref /teams?teamId=id in web
- (DONE) del team: confirmation modal requiring typed "delete"
- (DONE) creating new team in another page (separate create view with back button)
- (DONE) member detail page: clicking a member opens a modal with all fields editable (name, email, jira, role, level, skills, capacity) + save/remove
6. (DONE) Google Driver tab: refer /settings/google-drive-setup in web
- BIG BUG: completely missing comparing with web. Check again
7. (DONE) Artifacts page: refer /artifacts?tab=generated in web
7.1 (DONE) Tab Catalog: refer /artifacts?tab=catalog
- (DONE) Create in new page pattern added. Del button is a follow-up (no backend deleteDefinition yet)
7.2 (DONE) Tab Storage: refer /artifacts?tab=storage
Two-column layout: project list left, storage config right.
Supabase active → info panel (no driver setup). Google Drive active → driver config form.
![alt text](image-11.png) (Follow the concept, UI can be adapt in style of desktop app)
7.3 (DONE) Tab Generated
/artifacts?tab=generated
![alt text](image-12.png)
(DONE) Local/Not Synced | Remote/Sync sub-tabs added
(DONE) Sync artifacts button added
(DONE) Collapsible grouping by project → provider → workflowRunId added
