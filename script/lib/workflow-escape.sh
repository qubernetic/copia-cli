# Sourced by the script/check-*.sh guards.
#
# workflow_escape copies stdin to stdout with the workflow-command encoding:
# % as %25, CR as %0D and LF as %0A, except for the final newline. The runner
# reads every log line as a possible workflow command, so a value printed
# through it cannot start a line of its own.
workflow_escape() {
  cr=$(printf '\r')
  sed -e 's/%/%25/g' -e "s/$cr/%0D/g" |
    sed -e ':a' -e '$!N' -e 's/\n/%0A/' -e 'ta'
}
