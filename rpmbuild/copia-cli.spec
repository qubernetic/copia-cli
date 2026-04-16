%global goipath github.com/qubernetic/copia-cli
%global debug_package %{nil}

Name:           copia-cli
Version:        0.0.0
Release:        1%{?dist}
Summary:        CLI for Copia — source control for industrial automation

License:        AGPL-3.0-only
URL:            https://%{goipath}

%ifarch x86_64
Source0:        https://%{goipath}/releases/download/v%{version}/%{name}_%{version}_linux_amd64.tar.gz
%endif
%ifarch aarch64
Source0:        https://%{goipath}/releases/download/v%{version}/%{name}_%{version}_linux_arm64.tar.gz
%endif

%description
copia-cli brings Copia repositories, issues, pull requests, and other
concepts to the terminal next to where you are already working with
git and your code.

%prep
%setup -c

%install
install -Dpm 0755 %{name} %{buildroot}%{_bindir}/%{name}
install -Dpm 0644 LICENSE %{buildroot}%{_licensedir}/%{name}/LICENSE

%files
%license LICENSE
%{_bindir}/%{name}

%changelog
