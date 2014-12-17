package io.v.jenkins.plugins.veyron_scm;

import org.apache.commons.io.FilenameUtils;
import org.apache.commons.io.filefilter.FileFileFilter;
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
import hudson.Proc;
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
import hudson.util.FormValidation;

import java.io.BufferedReader;
import java.io.File;
import java.io.IOException;
import java.io.InputStreamReader;
import java.io.PrintWriter;
import java.util.ArrayList;
import java.util.Arrays;
import java.util.List;
import java.util.Map;

import javax.xml.parsers.DocumentBuilder;
import javax.xml.parsers.DocumentBuilderFactory;

/**
 * Veyron SCM Jenkins plugin.
 */
public class VeyronSCM extends SCM {
  private static final String LOG_PREFIX = "[Veyron-SCM]";

  /**
   * The minimum version of veyron tool to support "-manifest" flag in "veyron project" command.
   */
  private static final int VEYRON_MIN_VERSION = 355;

  /**
   * This field will automatically get the content of the VEYRON_ROOT text field in the UI.
   *
   * See: resources/io/v/jenkins/plugins/veyron_scm/VeyronSCM/config.jelly.
   */
  private String veyronRootInput;

  /**
   * This field will automatically get the content of the Manifest text field in the UI.
   *
   * See: resources/io/v/jenkins/plugins/veyron_scm/VeyronSCM/config.jelly.
   */
  private String manifestInput;

  /**
   * A wrapper class for storing result data of a command run.
   */
  private static final class CommandResult {
    private List<String> stdoutLines;
    private List<String> stderrLines;
    private int exitCode;

    public CommandResult(List<String> stdoutLines, List<String> stderrLines, int exitCode) {
      this.stdoutLines = stdoutLines;
      this.stderrLines = stderrLines;
      this.exitCode = exitCode;
    }

    public String getStdout() {
      return StringUtils.join(stdoutLines.toArray(), "\n");
    }

    public String getStderr() {
      return StringUtils.join(stderrLines.toArray(), "\n");
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
  public VeyronSCM(String veyronRootInput, String manifestInput) {
    this.veyronRootInput = veyronRootInput;
    this.manifestInput = manifestInput;
  }

  /**
   * This is required to bind veyronRootInput variable to the corresponding UI element.
   */
  public String getVeyronRootInput() {
    return veyronRootInput;
  }

  /**
   * This is required to bind the manifestInput variable to the corresponding UI element.
   */
  public String getManifestInput() {
    return manifestInput;
  }

  /**
   * We don't need to implement this method since we handle revision recording ourselves through the
   * veyron tool.
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

    // Run "veyron project poll <JOB_NAME>".
    String jobName = lastBuild.getEnvironment(listener).get("JOB_NAME");
    String workspaceDir = workspace.getRemote();
    String veyronBin = getVeyronBin(workspaceDir);

    // Check "veyron" tool's version to decide whether to add "-manifest" flag.
    List<String> pollCommandAndArgs = new ArrayList<String>(Arrays.asList(veyronBin, "project",
        String.format("-manifest=%s", manifestInput), "poll", jobName));
    List<String> checkVeyronVersionCommandAndArgs =
        new ArrayList<String>(Arrays.asList(veyronBin, "version"));
    CommandResult cr = runCommand(workspaceDir,
        launcher,
        listener,
        true,
        checkVeyronVersionCommandAndArgs,
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
      if (version > 0 && version < VEYRON_MIN_VERSION) {
        pollCommandAndArgs =
            new ArrayList<String>(Arrays.asList(veyronBin, "project", "poll", jobName));
      }
    } else {
      printf(listener, "command \"%s\" failed.\n", getCommand(checkVeyronVersionCommandAndArgs));
    }

    cr = runCommand(workspaceDir,
        launcher,
        listener,
        true,
        pollCommandAndArgs,
        lastBuild.getEnvironment(listener));
    if (cr.getExitCode() != 0) {
      printf(listener, "Polling command \"%s\" failed.\n", getCommand(pollCommandAndArgs));
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
    // Run "init-veyron.sh" script.
    String workspaceDir = workspace.getRemote();
    String home = build.getEnvironment(listener).get("HOME");
    List<String> initVeyronCommandAndArgs =
        new ArrayList<String>(Arrays.asList(joinPath(home, "scripts", "init-veyron.sh")));
    CommandResult cr = runCommand(workspaceDir,
        launcher,
        listener,
        true,
        initVeyronCommandAndArgs,
        build.getEnvironment(listener));
    if (cr.getExitCode() != 0) {
      printf(listener, "Init veyron script \"%s\" failed.\n", getCommand(initVeyronCommandAndArgs));
      return false;
    }

    // Run "veyron goext distclean".
    String veyronBin = getVeyronBin(workspaceDir);
    List<String> distcleanCommandAndArgs =
        new ArrayList<String>(Arrays.asList(veyronBin, "goext", "distclean"));
    cr = runCommand(workspaceDir,
        launcher,
        listener,
        true,
        distcleanCommandAndArgs,
        build.getEnvironment(listener));
    if (cr.getExitCode() != 0) {
      printf(listener, "Command \"%s\" failed.\n", getCommand(distcleanCommandAndArgs));
      return false;
    }

    // Run "veyron update -manifest=<manifest>".
    List<String> updateCommandAndArgs = new ArrayList<String>(
        Arrays.asList(veyronBin, "update", String.format("-manifest=%s", manifestInput), "-gc"));
    cr = runCommand(workspaceDir,
        launcher,
        listener,
        true,
        updateCommandAndArgs,
        build.getEnvironment(listener));
    if (cr.getExitCode() != 0) {
      printf(listener, "Update command \"%s\" failed.\n", getCommand(updateCommandAndArgs));
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

      String curGitDir = joinPath(getVeyronRoot(workspaceDir), snapshot.getRelativePath(), ".git");
      List<String> gitLogCommandAndArgs = new ArrayList<String>(Arrays.asList("git",
          String.format("--git-dir=%s", curGitDir),
          "log",
          "--format=raw",
          "--raw",
          "--no-merges",
          "--no-abbrev",
          String.format("%s..master", snapshot.getRevision())));
      cr = runCommand(workspaceDir,
          launcher,
          listener,
          false,
          gitLogCommandAndArgs,
          build.getEnvironment(listener));
      if (cr.getExitCode() != 0) {
        printf(listener, "\"%s\" failed.\n%s\n", getCommand(gitLogCommandAndArgs), cr.getStderr());
        continue;
      }
      String commitRaw = cr.getStdout();
      if (!commitRaw.isEmpty()) {
        changelogWriter.println(commitRaw);
      }
    }
    changelogWriter.close();
    printf(listener, "OK\n");

    // Create a VeyronBuildData to store Veyron related data for this build, and add it to the
    // current build object.
    String buildCopLDAP = "";
    List<String> buildCopCommandAndArgs =
        new ArrayList<String>(Arrays.asList(veyronBin, "buildcop"));
    cr = runCommand(workspaceDir,
        launcher,
        listener,
        false,
        buildCopCommandAndArgs,
        build.getEnvironment(listener));
    if (cr.getExitCode() != 0) {
      printf(listener, "Command \"%s\" failed.\n", getCommand(buildCopCommandAndArgs));
    } else {
      buildCopLDAP = cr.getStdout();
    }
    build.addAction(new VeyronBuildData(buildCopLDAP, snapshots));

    return true;
  }

  @Override
  public ChangeLogParser createChangeLogParser() {
    return new GitChangeLogParser(false);
  }

  @Extension
  public static final class DescriptorImpl extends SCMDescriptor<VeyronSCM> {
    public DescriptorImpl() {
      super(VeyronSCM.class, null);
    }

    @Override
    public String getDisplayName() {
      return "Veyron SCM";
    }

    /**
     * Validates "veyronRootInput" field to make sure it is not empty.
     */
    public FormValidation doCheckVeyronRootInput(@QueryParameter String value) {
      if (value.isEmpty()) {
        return FormValidation.error("VEYRON_ROOT cannot be empty.");
      }
      return FormValidation.ok();
    }
  }

  private CommandResult runCommand(String workspaceDir,
      Launcher launcher,
      TaskListener listener,
      boolean verbose,
      List<String> commandAndArgs,
      Map<String, String> env) {
    List<String> stdoutLines = new ArrayList<String>();
    List<String> stderrLines = new ArrayList<String>();
    int exitCode = -1;
    if (verbose) {
      printf(listener, "Running command: %s.\n", getCommand(commandAndArgs));
    }
    try {
      ProcStarter ps = launcher.new ProcStarter();
      env.put("VEYRON_ROOT", getVeyronRoot(workspaceDir));
      ps.envs(env).cmds(commandAndArgs).pwd(workspaceDir).quiet(true).readStdout().readStderr();
      Proc proc = launcher.launch(ps);
      BufferedReader bri = new BufferedReader(new InputStreamReader(proc.getStdout()));
      BufferedReader bre = new BufferedReader(new InputStreamReader(proc.getStderr()));
      String line;
      while ((line = bri.readLine()) != null) {
        if (verbose) {
          printf(listener, "%s\n", line);
        }
        stdoutLines.add(line);
      }
      bri.close();
      while ((line = bre.readLine()) != null) {
        if (verbose) {
          printf(listener, "%s\n", line);
        }
        stderrLines.add(line);
      }
      bre.close();
      exitCode = proc.join();
    } catch (Exception e) {
      e.printStackTrace(listener.getLogger());
    }
    return new CommandResult(stdoutLines, stderrLines, exitCode);
  }

  /**
   * Gets the latest snapshot file from $VEYRON_ROOT/.update_history directory.
   */
  private FilePath getLatestSnapshotFile(FilePath workspace, TaskListener listener) {
    FilePath updateHistoryDir = new FilePath(workspace.getChannel(),
        joinPath(getVeyronRoot(workspace.getRemote()), ".update_history"));
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
   * Gets the full path of VEYRON_ROOT.
   */
  private String getVeyronRoot(String workspaceDir) {
    if (veyronRootInput.startsWith(File.separator)) {
      return veyronRootInput;
    } else {
      return joinPath(workspaceDir, veyronRootInput);
    }
  }

  /**
   * Gets the full path of the veyron tool.
   */
  private String getVeyronBin(String workspaceDir) {
    return joinPath(getVeyronRoot(workspaceDir), "bin", "veyron");
  }

  /**
   * A helper function to print messages with LOG_PREFIX to the Jenkins console.
   */
  private void printf(TaskListener listener, String format, Object... args) {
    listener.getLogger().print(LOG_PREFIX + " ");
    listener.getLogger().printf(format, args);
  }
}
