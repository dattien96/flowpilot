- start 1 workflow
- The timeout set for project is 5m
- Wait for timeout 

We saw that
- UI count for the active session is 0 - correct
- Session state is : completed - correct
- 
- 
But after check OS thread i still see that process

Wait for more a bit time then the process was killed
I think due to the golang background process run

BUt the timming is not correct, it does not kill process at the same time sync with UI/supabase