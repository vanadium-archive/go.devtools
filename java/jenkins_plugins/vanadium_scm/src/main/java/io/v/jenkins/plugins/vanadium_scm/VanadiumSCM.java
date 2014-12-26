package io.v.jenkins.plugins.vanadium_scm;

import org.apache.commons.io.FilenameUtils;
import org.apache.commons.io.filefilter.FileFileFilter;
import org.apache.commons.io.output.NullOutputStream;
import org.apache.commons.lang.StringUtils;
import org.kohsuke.stapler.DataBoundConstructor;
import org.kohsuke.stapler.QueryParameter;
import org.w3c.dom.Document;
import org.w3c.dom.Element;
import org.w3c.dom.Node;
import org.w3c.dom.NodeList;

import hudson.Extension;
import hudson.FilePath;
import hudson.Launcher;
import hudson.Launcher.ProcStarter;
import hudson.model.BuildListener;
import hudson.model.TaskListener;
import hudson.model.AbstractBuild;
import hudson.model.AbstractProject;
import hudson.scm.ChangeLogParser;
import hudson.scm.PollingResult;
import hudson.scm.PollingResult.Change;
import hudson.scm.SCMDescriptor;
import hudson.scm.SCMRevisionState;
import hudson.scm.SCM;
import hudson.util.ForkOutputStream;
import hudson.util.FormValidation;

import java.io.ByteArrayOutputStream;
import java.io.File;
import java.io.IOException;
import java.io.OutputStream;
import java.io.PrintWriter;
import java.util.ArrayList;
import java.util.Arrays;
import java.util.List;
import java.util.Map;
import java.util.concurrent.TimeUnit;

import javax.xml.parsers.DocumentBuilder;
import javax.xml.parsers.DocumentBuilderFactory;

/**
 * Vanadium SCM Jenkins plugin.
 */
public class VanadiumSCM extends SCM {
  private static final String LOG_PREFIX = "[Vanadium-SCM]";

  /**
   * The minimum version of v23 tool to support "-manifest" flag in "v23 project" command.
   */
  private static final int V23_MIN_VERSION = 355;

  /**
   * Number of times to attempt "v23 update" command.
   */
  private static final int V23_UPDATE_ATTEMPTS = 3;

  /**
   * Global command timeout in minutes.
   */
  private static final int CMD_TIMEOUT_MINUTES = 10;

  /**
   * This field will automatically get the content of the VANADIUM_ROOT text field in the UI.
   *
   * See: resources/io/v/jenkins/plugins/vanadium_scm/VanadiumSCM/config.jelly.
   */
  private String vanadiumRootInput;

  /**
   * This field will automatically get the content of the Manifest text field in the UI.
   *
   * See: resources/io/v/jenkins/plugins/vanadium_scm/VanadiumSCM/config.jelly.
   */
  private String manifestInput;

  /**
   * A wrapper class for storing result data of a command run.
   */
  private static final class CommandResult {
    private final String stdout;
    private final int exitCode;

    public CommandResult(String stdout, int exitCode) {
      this.stdout = stdout;
      this.exitCode = exitCode;
    }

    public String getStdout() {
      return stdout;
    }

    public int getExitCode() {
      return exitCode;
    }
  }

  /**
   * A wrapper class for storing project snapshot data.
   */
  public static class ProjectSnapshot {
    private String name;
    private String relativePath;
    private String protocol;
    private String revision;

    public ProjectSnapshot(String name, String relativePath, String protocal, String revision) {
      this.name = name;
      this.relativePath = relativePath;
      this.protocol = protocal;
      this.revision = revision;
    }

    public String getName() {
      return name;
    }

    public String getShortName() {
      return FilenameUtils.getName(name);
    }

    public String getRelativePath() {
      return relativePath;
    }

    public String getProtocol() {
      return protocol;
    }

    public String getRevision() {
      return revision;
    }
  }

  @DataBoundConstructor
  public VanadiumSCM(String vanadiumRootInput, String manifestInput) {
    this.vanadiumRootInput = vanadiumRootInput;
    this.manifestInput = manifestInput;
  }

  /**
   * This is required to bind vanadiumRootInput variable to the corresponding UI element.
   */
  public String getVanadiumRootInput() {
    return vanadiumRootInput;
  }

  /**
   * This is required to bind the manifestInput variable to the corresponding UI element.
   */
  public String getManifestInput() {
    return manifestInput;
  }

  /**
   * We don't need to implement this method since we handle revision recording ourselves through the
   * v23 tool.
   */
  @Override
  public SCMRevisionState calcRevisionsFromBuild(AbstractBuild<?, ?> build, Launcher launcher,
      TaskListener listener) throws IOException, InterruptedException {
    return SCMRevisionState.NONE;
  }

  /**
   * Polling should be implemented in this method.
   */
  @Override
  protected PollingResult compareRemoteRevisionWith(AbstractProject<?, ?> project,
      Launcher launcher, FilePath workspace, TaskListener listener, SCMRevisionState baseline)
      throws IOException, InterruptedException {
    // If the project has never been built before, force a build and skip polling.
    final AbstractBuild<?, ?> lastBuild = project.getLastBuild();
    if (lastBuild == null) {
      printf(listener, "No previous build. Forcing an initial build.\n");
      return PollingResult.BUILD_NOW;
    }

    // Run "v23 project poll <JOB_NAME>".
    String jobName = lastBuild.getEnvironment(listener).get("JOB_NAME");
    String workspaceDir = workspace.getRemote();
    String v23Bin = getV23Bin(workspaceDir);

    // Check "v23" tool's version to decide whether to add "-manifest" flag.
    List<String> pollCommandAndArgs = new ArrayList<String>(Arrays.asList(v23Bin, "project",
        String.format("-manifest=%s", manifestInput), "poll", jobName));
    List<String> checkV23VersionCommandAndArgs =
        new ArrayList<String>(Arrays.asList(v23Bin, "version"));
    CommandResult cr = runCommand(workspaceDir, launcher, true, checkV23VersionCommandAndArgs,
        lastBuild.getEnvironment(listener));
    if (cr.getExitCode() == 0) {
      String[] parts = cr.getStdout().split(" ");
      String strVersion = parts[parts.length - 1];
      int version = -1;
      try {
        version = Integer.parseInt(strVersion);
      } catch (NumberFormatException e) {
        e.printStackTrace(listener.getLogger());
      }
      if (version > 0 && version < V23_MIN_VERSION) {
        pollCommandAndArgs =
            new ArrayList<String>(Arrays.asList(v23Bin, "project", "poll", jobName));
      }
    }

    cr = runCommand(workspaceDir, launcher, true, pollCommandAndArgs,
        lastBuild.getEnvironment(listener));
    if (cr.getExitCode() != 0) {
      return new PollingResult(Change.NONE);
    }

    // Check output.
    return new PollingResult(cr.getStdout().isEmpty() ? Change.NONE : Change.SIGNIFICANT);
  }

  /**
   * Code checkout should be implemented in this method.
   */
  @Override
  public boolean checkout(AbstractBuild<?, ?> build, Launcher launcher, FilePath workspace,
      BuildListener listener, File changelogFile) throws IOException, InterruptedException {
    // Run "init-vanadium.sh" script.
    String workspaceDir = workspace.getRemote();
    String home = build.getEnvironment(listener).get("HOME");
    List<String> initVanadiumCommandAndArgs =
        new ArrayList<String>(Arrays.asList(joinPath(home, "scripts", "init-vanadium.sh")));
    CommandResult cr = runCommand(workspaceDir, launcher, true, initVanadiumCommandAndArgs,
        build.getEnvironment(listener));
    if (cr.getExitCode() != 0) {
      return false;
    }

    // Run "v23 goext distclean".
    String v23Bin = getV23Bin(workspaceDir);
    List<String> distcleanCommandAndArgs =
        new ArrayList<String>(Arrays.asList(v23Bin, "goext", "distclean"));
    cr = runCommand(workspaceDir, launcher, true, distcleanCommandAndArgs,
        build.getEnvironment(listener));
    if (cr.getExitCode() != 0) {
      return false;
    }

    // Run "v23 update -manifest=<manifest>".
    List<String> updateCommandAndArgs = new ArrayList<String>(
        Arrays.asList(v23Bin, "update", String.format("-manifest=%s", manifestInput), "-gc"));
    for (int i = 0; i < V23_UPDATE_ATTEMPTS; i++) {
      printf(listener, String.format("Attempt #%d:\n", i + 1));
      cr = runCommand(workspaceDir, launcher, true, updateCommandAndArgs,
          build.getEnvironment(listener));
      if (cr.getExitCode() == 0) {
        break;
      }
    }
    if (cr.getExitCode() != 0) {
      return false;
    }

    // If there is no previous build, skip generating changelog.
    if (build.getProject().getLastBuild() == null) {
      printf(listener, "No previous build. Skip generating changelog.\n");
      return true;
    }

    // Parse the latest snapshot file.
    FilePath latestSnapshotFile = getLatestSnapshotFile(workspace, listener);
    if (latestSnapshotFile == null) {
      printf(listener, "Failed to get snapshot file.\n");
      return false;
    }
    List<ProjectSnapshot> snapshots = parseSnapshotFile(latestSnapshotFile, listener);
    if (snapshots == null) {
      printf(listener, "Failed to parse the snapshot file: %s\n", latestSnapshotFile.getRemote());
      return false;
    }

    // For each snapshot, get all changes between snapshot's revision and master using "git log",
    // and output the raw changes to the changelog file.
    printf(listener, "Generating changelog file...\n");
    PrintWriter changelogWriter = new PrintWriter(changelogFile);
    for (ProjectSnapshot snapshot : snapshots) {
      // TODO(jingjin): Support other protocols.
      if (!snapshot.getProtocol().equals("git")) {
        continue;
      }

      String curGitDir = joinPath(getVanadiumRoot(workspaceDir), snapshot.getRelativePath(), ".git");
      List<String> gitLogCommandAndArgs = new ArrayList<String>(Arrays.asList("git",
          String.format("--git-dir=%s", curGitDir),
          "log",
          "--format=raw",
          "--raw",
          "--no-merges",
          "--no-abbrev",
          String.format("%s..master", snapshot.getRevision())));
      cr = runCommand(workspaceDir, launcher, false, gitLogCommandAndArgs,
          build.getEnvironment(listener));
      if (cr.getExitCode() != 0) {
        continue;
      }
      String commitRaw = cr.getStdout();
      if (!commitRaw.isEmpty()) {
        changelogWriter.println(commitRaw);
      }
    }
    changelogWriter.close();
    printf(listener, "OK\n");

    // Create a VanadiumBuildData to store Vanadium related data for this build, and add it to the
    // current build object.
    String buildCopLDAP = "";
    List<String> buildCopCommandAndArgs =
        new ArrayList<String>(Arrays.asList(v23Bin, "buildcop"));
    cr = runCommand(workspaceDir, launcher, false, buildCopCommandAndArgs,
        build.getEnvironment(listener));
    if (cr.getExitCode() == 0) {
      buildCopLDAP = cr.getStdout();
    }
    build.addAction(new VanadiumBuildData(buildCopLDAP, snapshots));

    return true;
  }

  @Override
  public ChangeLogParser createChangeLogParser() {
    return new GitChangeLogParser(false);
  }

  @Extension
  public static final class DescriptorImpl extends SCMDescriptor<VanadiumSCM> {
    public DescriptorImpl() {
      super(VanadiumSCM.class, null);
    }

    @Override
    public String getDisplayName() {
      return "Vanadium SCM";
    }

    /**
     * Validates "vanadiumRootInput" field to make sure it is not empty.
     */
    public FormValidation doCheckVanadiumRootInput(@QueryParameter String value) {
      if (value.isEmpty()) {
        return FormValidation.error("VANADIUM_ROOT cannot be empty.");
      }
      return FormValidation.ok();
    }
  }

  private CommandResult runCommand(String workspaceDir, Launcher launcher, boolean verbose,
      List<String> commandAndArgs, Map<String, String> env) {
    String stdout = "";
    String stderr = "";
    int exitCode = -1;
    TaskListener listener = launcher.getListener();
    if (verbose) {
      printf(listener, "Running command: %s.\n", getCommand(commandAndArgs));
    }
    try {
      env.put("VANADIUM_ROOT", getVanadiumRoot(workspaceDir));
      ByteArrayOutputStream bosStdout = new ByteArrayOutputStream();
      ByteArrayOutputStream bosStderr = new ByteArrayOutputStream();
      OutputStream osStdout = new ForkOutputStream(
          verbose ? listener.getLogger() : NullOutputStream.NULL_OUTPUT_STREAM, bosStdout);
      OutputStream osStderr = new ForkOutputStream(
          verbose ? listener.getLogger() : NullOutputStream.NULL_OUTPUT_STREAM, bosStderr);
      ProcStarter ps = launcher
          .launch()
          .envs(env)
          .cmds(commandAndArgs)
          .pwd(workspaceDir)
          .quiet(true)
          .stdout(osStdout)
          .stderr(osStderr);
      exitCode = ps.start().joinWithTimeout(CMD_TIMEOUT_MINUTES, TimeUnit.MINUTES, listener);
      stdout = bosStdout.toString();
      stderr = bosStderr.toString();
      bosStdout.close();
      bosStderr.close();
    } catch (Exception e) {
      e.printStackTrace(listener.getLogger());
    }
    if (exitCode != 0) {
      printf(listener, "Command '%s' failed.\n", getCommand(commandAndArgs));
      if (!verbose) {
        printf(listener, "%s\n%s\n", stdout, stderr);
      }
    }
    return new CommandResult(stdout, exitCode);
  }

  /**
   * Gets the latest snapshot file from $VANADIUM_ROOT/.update_history directory.
   */
  private FilePath getLatestSnapshotFile(FilePath workspace, TaskListener listener) {
    FilePath updateHistoryDir = new FilePath(workspace.getChannel(),
        joinPath(getVanadiumRoot(workspace.getRemote()), ".update_history"));
    FilePath latest = null;
    try {
      List<FilePath> files = updateHistoryDir.list(FileFileFilter.FILE);
      long lastMod = Long.MIN_VALUE;
      for (FilePath file : files) {
        long curLastModified = file.lastModified();
        if (curLastModified > lastMod) {
          latest = file;
          lastMod = curLastModified;
        }
      }
    } catch (Exception e) {
      e.printStackTrace(listener.getLogger());
    }
    return latest;
  }

  /**
   * Parses a given snapshot file.
   */
  private List<ProjectSnapshot> parseSnapshotFile(FilePath snapshotFile, TaskListener listener) {
    List<ProjectSnapshot> snapshots = new ArrayList<ProjectSnapshot>();
    try {
      DocumentBuilderFactory dbf = DocumentBuilderFactory.newInstance();
      DocumentBuilder db = dbf.newDocumentBuilder();
      Document doc = db.parse(snapshotFile.read());
      NodeList projects = doc.getElementsByTagName("project");
      for (int i = 0; i < projects.getLength(); i++) {
        Node project = projects.item(i);
        if (project.getNodeType() == Node.ELEMENT_NODE) {
          Element eleProject = (Element) project;
          snapshots.add(new ProjectSnapshot(eleProject.getAttribute("name"),
              eleProject.getAttribute("path"), eleProject.getAttribute("protocol"),
              eleProject.getAttribute("revision")));
        }
      }
    } catch (Exception e) {
      e.printStackTrace(listener.getLogger());
      return null;
    }
    return snapshots;
  }

  /**
   * A helper method to join the given path components.
   */
  private String joinPath(String... paths) {
    return StringUtils.join(paths, File.separator);
  }

  /**
   * A helper method to get a formatted string of a list that has command and its arguments.
   */
  private String getCommand(List<String> commandAndArgs) {
    return StringUtils.join(commandAndArgs.toArray(), " ");
  }

  /**
   * Gets the full path of VANADIUM_ROOT.
   */
  private String getVanadiumRoot(String workspaceDir) {
    if (vanadiumRootInput.startsWith(File.separator)) {
      return vanadiumRootInput;
    } else {
      return joinPath(workspaceDir, vanadiumRootInput);
    }
  }

  /**
   * Gets the full path of the v23 tool.
   */
  private String getV23Bin(String workspaceDir) {
    return joinPath(getVanadiumRoot(workspaceDir), "bin", "v23");
  }

  /**
   * A helper function to print messages with LOG_PREFIX to the Jenkins console.
   */
  private void printf(TaskListener listener, String format, Object... args) {
    listener.getLogger().print(LOG_PREFIX + " ");
    listener.getLogger().printf(format, args);
  }
}
