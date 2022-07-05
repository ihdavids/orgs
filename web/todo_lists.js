var todo_lists = {
  "alltasks": "!IsArchived() && !IsProject() && IsTodo()// Lists all tasks found",
  "donetasks": "!IsArchived() && !IsProject() && HasAStatus() && !IsActive()// Lists all finished tasks",
  "today":    "!IsProject() && IsTodo() && Today()// Tasks scheduled for today",
  "untagged": "!IsProject() && IsTodo() && NoTags()// Tasks missing tags",
  "next":     "!IsProject() && IsTodo() && IsStatus('NEXT')// Tasks marked as next",
  "projects": "IsProject()// All Projects",
  "romark": "IsTask() && (IsPartOfProject('.*ROMARK.*') || HasTags('RoMark'))// Anything tagged as RoMark",
  "backlog": "IsTask() && (HasTags('Backlog'))// Anything marked for my backlog",
  "active": "IsTask() && (HasTags('ACTIVE'))// Anything marked as an active task",
  "atest": "!IsProject() && IsTodo() && !IsArchived() && MatchHeadline('A.*')"
}