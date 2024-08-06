# Instructions to generate the demo in .mp4

# Run:
# $ vhs etcd-druid-demo-realtime.tape

# The generated video will be extremely long since the etcd cluster takes more than 2 minutes for all members to become Ready.

# Use ffmpeg to speed up those 2 minutes.

# ENSURE to run the first ffmpeg command with the flags in the specified order.
# If this is not done, the first few seconds of the demo would get skipped-
# since the order of the flags matters to find the first key frame in ffmpeg.
# Let the timestamp from which you want the playback to be sped up be H1:M1:S1,
# and the timestamp till which you want the playback to be sped up be H2:M2:S2,
# and the speed multiplier be SPEED,
# and the end time of the video be END.

# Run:
# $ cp etcd-druid-demo-realtime.mp4 input.mp4
# $ ffmpeg -ss 0 -i input.mp4 -to H1:M1:S1 -c copy part1.mp4
# $ ffmpeg -i input.mp4 -ss H1:M1:S1 -to 00:03:34 -c copy part2.mp4
# $ ffmpeg -i input.mp4 -ss H2:M2:S2 -to END -c copy part3.mp4
# $ ffmpeg -i part2.mp4 -filter:v "setpts=PTS/SPEED" part2_fast.mp4
# $ cat << EOF > concat_list.txt
# file 'part1.mp4'
# file 'part2_fast.mp4'
# file 'part3.mp4'
# EOF
# $ ffmpeg -f concat -safe 0 -i concat_list.txt -c copy output.mp4
# $ cp output.mp4 etcd-druid-demo.mp4

Output etcd-druid-demo-realtime.mp4

Require make
Require kubectl
Require tmux

Set FontSize 16
Set FontFamily "JetBrainsMono Nerd Font"
Set Width 1200
Set Height 800
Set WindowBar Colorful
Set CursorBlink true
Set TypingSpeed 75ms
Set WaitTimeout 30s

Type "# Target any kubernetes cluster."
Sleep 130ms
Enter

Type "# For this demo, we set up a local kubernetes cluster using kind, through the `kind-up` make target:"
Sleep 130ms
Enter

Type "make kind-up"
Sleep 130ms
Enter
Wait

Type "# Target this kind cluster by exporting KUBECONFIG and pointing it to the kubeconfig path of the kind cluster as shown above."
Sleep 130ms
Enter

Type@50ms "export KUBECONFIG=hack/e2e-test/infrastructure/kind/kubeconfig"
Sleep 260ms
Enter

Type "# We now have to install the CRDs and deploy etcd-druid."
Sleep 130ms
Enter

Type "# The `deploy` make target installs CRDs, and creates a Deployment for etcd-druid in the default namespace:"
Sleep 130ms
Enter

Type "make deploy"
Sleep 130ms
Enter
# a magic number that makes it work, things break otherwise
# Find this number with `$ time make deploy` and add some buffer
Sleep 12s

Type "kubectl get pod # Checking the etcd-druid pod."
Sleep 130ms
Enter
Wait

Type "# etcd-druid has now been deployed; and is ready to provision, reconcile, and monitor etcd clusters."
Sleep 130ms
Enter

Type "# To create an etcd-cluster, just define an etcd CR and deploy it."
Sleep 3s
Enter

# setup tmux
Hide
  Type "tmux -f /dev/null -L test new-session -- bash" Enter
  Type "tmux split-window -d -- bash && \" Enter
  Type "tmux set status && \" Enter
  Type 'tmux setw pane-border-style "fg=0" && \' Enter
  Type 'tmux setw pane-active-border-style "fg=0"' Enter
  Sleep 0.5
  Ctrl+L
  Sleep 1
Show

# pane 1
  Type "# Using a pre-defined sample, config/samples/druid_v1alpha1_etcd.yaml, which creates a 3 member etcd-cluster."
  Sleep 130ms
  Enter

  Type "kubectl apply -f config/samples/druid_v1alpha1_etcd.yaml"
  Sleep 130ms
# pane 1

# switch pane to 2
Hide
Ctrl+B
Type o
Show

# pane 2
  Type "# Meanwhile, keeping a watch on some of the resources being provisioned by etcd-druid:"
  Enter
  Sleep 130ms
  Type "watch --interval 5 kubectl get etcd,sts,pod"
  Enter
  Sleep 130ms
# pane 2

# switch pane to 1
Hide
Ctrl+B
Type o
Show

# pane 1
  Enter
  Sleep 210s

  Type "# The etcd cluster is now ready."
  Sleep 130ms
  Enter

  Type "# Bringing the etcd cluster down is done by simply deleting the CR:"
  Sleep 130ms
  Enter

  Type "kubectl delete etcd etcd-test"
  Sleep 130ms
  Enter
  Sleep 10s

  Type "# All resources provisioned by etcd-druid have been brought down, as can be seen below."
  Sleep 130ms
  Enter
# pane 1

# kill pane 2
Hide
Ctrl+B
Type o
Ctrl+C
Type "tmux kill-pane"
Enter
Show

Type "# Shutting down the kind cluster:"
Sleep 130ms
Enter

Type "make kind-down"
Sleep 130ms
Enter
Sleep 5s

Hide
Type "tmux kill-session"
Sleep 130ms
Enter
Show
