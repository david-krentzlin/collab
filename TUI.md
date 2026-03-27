# Goal

Add a TUI view to show pairing conversations.
The layout should be like this:

```
| Tasks  |    Conversations   |
-----------------------------
| taska  | - thread           |
| *taskb |
```

So a panel to the left which shows all the known tasks.
It monitors all and marks the ones that have fresh conversations with an icon.
On the right we show the conversations of the selected task on the left.
We use a threaded rendering, which clearly shows which agent said what.
We use a viewport to enable scrolling and we follow by default, meaning when new data arrives we display it.

# Implementation

We build this feature in multiple steps:

1. Basic layout 
Just setup the basic TUI with bubbletea v2 and lipgloss as a fullscreen application with alternate screen.
Render the layout without anything in it yet, make sure we respond to terminal resizes.
Make sure we can exit the app again.

2. Add list of tasks to the left
Add the list of tasks that are currently known.
Add new tasks once they arrive.
Update tasks entries with icon when new data arrives
Task should contain number of messages in it in the description

3. Add task conversation view
Add the view port that renders the content without fancy formatting
Just the data and follows with auto scroll

4. Add formatting to the conversation view.
clearly denote the agent who speaks and try to establish thread marks.
